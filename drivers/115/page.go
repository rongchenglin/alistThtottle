package _115

import (
	"context"
	"fmt"
	"path"
	"strings"
	"time"

	driver115 "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/Xhofe/go-cache"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
)

var _ driver.PagedReader = (*Pan115)(nil)
var _ driver.Getter = (*Pan115)(nil)

// ListPage performs exactly one bounded upstream request. It never fills a
// short page or calls the full-directory List contract.
func (d *Pan115) ListPage(ctx context.Context, dir model.Obj, args model.PageListArgs) (*model.PageList, error) {
	if args.Offset < 0 || args.Limit <= 0 || args.Limit > driver115.MaxDirPageLimit || args.Offset > (1<<63-1)-args.Limit {
		return nil, errors.New("invalid 115 page range")
	}
	d.objectMu.Lock()
	generation := d.objectGeneration
	d.objectMu.Unlock()
	order := driver115.FileOrderByTime
	switch args.OrderBy {
	case "", "modified":
	case "name":
		order = driver115.FileOrderByName
	case "size":
		order = driver115.FileOrderBySize
	default:
		return nil, errors.New("unsupported 115 page sort")
	}
	asc := "1"
	switch args.OrderDirection {
	case "", "asc":
	case "desc":
		asc = "0"
	default:
		return nil, errors.New("unsupported 115 page sort direction")
	}
	if d.ExtractFolder == "back" {
		return nil, errors.Wrap(errs.NotSupport, "115 upstream pagination cannot place folders last")
	}
	mix := "1"
	if d.ExtractFolder == "front" {
		mix = "0"
	}
	result, err := d.getFilesPageWithThumb(ctx, dir.GetID(), driver115.ApiFileList, args.Limit, args.Offset, map[string]string{"o": order, "asc": asc, "fc_mix": mix, "natsort": "1"})
	if err != nil {
		return nil, err
	}
	if result.Count < 0 || int64(len(result.Files)) > args.Limit {
		return nil, driver115.ErrUnexpected
	}
	if len(result.Files) == 0 && args.Offset < int64(result.Count) {
		return nil, errors.Wrap(driver115.ErrUnexpected, "empty 115 page before end of directory")
	}
	objects := make([]model.Obj, 0, len(result.Files))
	for i := range result.Files {
		obj := fileObjFromInfo(&result.Files[i])
		if dir.GetPath() != "" {
			obj.Path = path.Join(dir.GetPath(), utils.MappingName(obj.GetName()))
		}
		objects = append(objects, &obj)
	}
	d.rememberPageObjects(dir.GetPath(), objects, generation)
	return &model.PageList{Objs: objects, Total: int64(result.Count), HasMore: args.Offset+args.Limit < int64(result.Count)}, nil
}

func (d *Pan115) objectCache() cache.ICache[model.Obj] {
	d.objectMu.Lock()
	defer d.objectMu.Unlock()
	if d.pageObjects == nil {
		d.pageObjects = cache.NewMemCache[model.Obj](cache.WithShards[model.Obj](4))
	}
	return d.pageObjects
}

// InvalidatePageObjects is also called by the generic page cache before a
// refresh or after a mutation. In-flight pages cannot repopulate older state.
func (d *Pan115) InvalidatePageObjects() {
	objects := d.objectCache()
	d.objectMu.Lock()
	defer d.objectMu.Unlock()
	d.objectGeneration++
	objects.Clear()
}

func (d *Pan115) rememberPageObjects(dirPath string, objects []model.Obj, generation uint64) {
	if dirPath == "" || d.CacheExpiration <= 0 {
		return
	}
	cached := d.objectCache()
	d.objectMu.Lock()
	defer d.objectMu.Unlock()
	if generation != d.objectGeneration {
		return
	}
	for _, obj := range objects {
		if file, ok := obj.(*FileObj); ok {
			cached.Set(path.Join(dirPath, utils.MappingName(file.GetName())), file.CloneObj(), cache.WithEx[model.Obj](time.Minute*time.Duration(d.CacheExpiration)))
		}
	}
}

// Display-name mappings need not be reversible. Reuse original names from
// visited pages; never guess an inverse mapping for a cold ambiguous path.
func (d *Pan115) nativeRelativePath(name string) (string, error) {
	parts := strings.Split(strings.TrimPrefix(name, "/"), "/")
	prefix := "/"
	for i, part := range parts {
		prefix = path.Join(prefix, part)
		if obj, ok := d.objectCache().Get(prefix); ok {
			parts[i] = obj.GetName()
			continue
		}
		for original, display := range conf.FilenameCharMap {
			if original != display && display != "" && strings.Contains(part, display) {
				return "", errors.Wrap(errs.NotSupport, "cold path contains a mapped name; open its parent page first")
			}
		}
	}
	return path.Join(parts...), nil
}

// Get uses a direct path lookup (or a bounded exact-name search for a file),
// never a complete parent listing, including for a cold deep link.
func (d *Pan115) Get(ctx context.Context, name string) (model.Obj, error) {
	if err := d.checkRequest(ctx); err != nil {
		return nil, err
	}
	name = utils.FixAndCleanPath(name)
	rootID := d.RootFolderID
	if rootID == "" {
		rootID = "0"
	}
	if name == "/" {
		return &model.Object{ID: rootID, Path: "/", Name: "root", IsFolder: true}, nil
	}
	if obj, ok := d.objectCache().Get(name); ok {
		if file, ok := obj.(*FileObj); ok {
			return file.CloneObj(), nil
		}
		return obj, nil
	}
	rootPath, err := d.rootPath(ctx, rootID)
	if err != nil {
		return nil, err
	}
	relative, err := d.nativeRelativePath(name)
	if err != nil {
		return nil, err
	}
	fullPath := path.Join(rootPath, relative)
	result := driver115.APIGetDirIDResp{}
	resp, err := d.client.Client.R().SetContext(ctx).SetQueryParam("path", fullPath).ForceContentType("application/json").SetResult(&result).Get(driver115.ApiDirName2CID)
	if err != nil {
		return nil, err
	}
	if result.State && string(result.CategoryID) != "" && string(result.CategoryID) != "0" {
		return &model.Object{ID: string(result.CategoryID), Path: name, Name: path.Base(name), IsFolder: true}, nil
	}
	if err = driver115.CheckErr(nil, &result, resp); err != nil && !errors.Is(err, driver115.ErrNotExist) && !errors.Is(err, driver115.ErrUnexpected) {
		return nil, err
	}
	parent, err := d.Get(ctx, path.Dir(name))
	if err != nil {
		return nil, err
	}
	if !parent.IsDir() {
		return nil, errs.NotFolder
	}
	// Search is bounded even if there are many similarly named results. Do not
	// scan other pages on a miss: that would recreate a hidden directory crawl.
	files := fileListRespWithThumb{}
	response, err := d.client.Client.R().SetContext(ctx).SetQueryParams(map[string]string{"cid": parent.GetID(), "search_value": path.Base(relative), "offset": "0", "limit": "50", "format": "json"}).ForceContentType("application/json").SetResult(&files).Get(driver115.ApiFileSearch)
	if err = driver115.CheckErr(err, &files, response); err != nil {
		return nil, err
	}
	for i := range files.Files {
		obj := fileObjFromInfo(&files.Files[i])
		if utils.MappingName(obj.GetName()) == path.Base(name) && obj.ParentID == parent.GetID() {
			obj.Path = name
			return &obj, nil
		}
	}
	if files.Count > len(files.Files) {
		return nil, errors.Wrap(errs.NotSupport, "bounded file lookup is ambiguous; open the containing directory page first")
	}
	return nil, errs.ObjectNotFound
}

func (d *Pan115) rootPath(ctx context.Context, rootID string) (string, error) {
	if rootID == "0" {
		return "/", nil
	}
	var result struct {
		driver115.BasicResp
		Name  string `json:"file_name"`
		Paths []struct {
			ID   driver115.IntString `json:"file_id"`
			Name string              `json:"file_name"`
		} `json:"paths"`
	}
	resp, err := d.client.Client.R().SetContext(ctx).SetQueryParam("cid", rootID).ForceContentType("application/json").SetResult(&result).Get(driver115.ApiFileStat)
	if err != nil {
		return "", err
	}
	if resp.StatusCode() != 200 {
		return "", fmt.Errorf("115 root lookup HTTP %d", resp.StatusCode())
	}
	if result.Name == "" || len(result.Paths) == 0 {
		return "", errors.Wrap(driver115.ErrUnexpected, "115 mount root has no path information")
	}
	parts := []string{"/"}
	for _, p := range result.Paths {
		if string(p.ID) != "0" {
			parts = append(parts, p.Name)
		}
	}
	if string(result.Paths[len(result.Paths)-1].ID) != rootID {
		parts = append(parts, result.Name)
	}
	for _, part := range parts[1:] {
		if strings.Contains(part, "/") {
			return "", errors.New("invalid 115 root path")
		}
	}
	return path.Join(parts...), nil
}
