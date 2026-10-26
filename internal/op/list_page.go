package op

import (
	"context"
	"fmt"
	"math"
	stdpath "path"
	"slices"
	"sync"
	"time"

	"github.com/Xhofe/go-cache"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/pkg/singleflight"
	"github.com/alist-org/alist/v3/pkg/utils"
	"github.com/pkg/errors"
)

var pageCache = cache.NewMemCache(cache.WithShards[*model.PageList](16))
var pageG singleflight.Group[*model.PageList]
var pageMu sync.Mutex
var pageGenerations = make(map[*model.Storage]uint64)

// Invalidate all pages of this instance, including descendants not seen by List.
// Generations also prevent pre-mutation in-flight calls from repopulating the cache.
func invalidatePageCache(storage driver.Driver) {
	if _, ok := storage.(driver.PagedReader); !ok {
		return
	}
	pageMu.Lock()
	pageGenerations[storage.GetStorage()]++
	pageMu.Unlock()
	if invalidator, ok := storage.(driver.PageObjectInvalidator); ok {
		invalidator.InvalidatePageObjects()
	}
}

func clonePage(page *model.PageList) *model.PageList {
	if page == nil {
		return nil
	}
	result := *page
	result.Objs = slices.Clone(page.Objs)
	for i, obj := range result.Objs {
		result.Objs[i] = clonePageObj(obj)
	}
	return &result
}

func clonePageObj(obj model.Obj) model.Obj {
	switch value := obj.(type) {
	case *model.ObjWrapName:
		cloned := *value
		cloned.Obj = clonePageObj(value.Obj)
		return &cloned
	case interface{ CloneObj() model.Obj }:
		return value.CloneObj()
	case *model.Object:
		cloned := *value
		return &cloned
	default:
		// Other optional paged drivers must return immutable object values.
		return obj
	}
}

// ListPage fetches only the requested upstream range. It never populates List's
// cache, sorts partial results, or invokes whole-directory update hooks.
func ListPage(ctx context.Context, storage driver.Driver, path string, args model.PageListArgs) (*model.PageList, error) {
	reader, ok := storage.(driver.PagedReader)
	if !ok {
		return nil, errs.NotImplement
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if args.Offset < 0 || args.Limit <= 0 || args.Offset > math.MaxInt64-args.Limit {
		return nil, errors.New("invalid page offset or limit")
	}
	if storage.Config().CheckStatus && storage.GetStorage().Status != WORK {
		return nil, errors.Errorf("storage not init: %s", storage.GetStorage().Status)
	}
	path = utils.FixAndCleanPath(path)
	if args.Refresh {
		invalidatePageCache(storage)
	}
	pageMu.Lock()
	generation := pageGenerations[storage.GetStorage()]
	key := fmt.Sprintf("%p:%d:%q:%d:%q:%d:%d:%q:%q", storage.GetStorage(), storage.GetStorage().ID, storage.GetStorage().MountPath, generation, path, args.Offset, args.Limit, args.OrderBy, args.OrderDirection)
	useCache := !storage.Config().NoCache && storage.GetStorage().CacheExpiration > 0
	if useCache {
		if page, found := pageCache.Get(key); found {
			pageMu.Unlock()
			return clonePage(page), nil
		}
	}
	pageMu.Unlock()
	result := pageG.DoChan(key, func() (*model.PageList, error) {
		if useCache {
			if page, found := pageCache.Get(key); found {
				return page, nil
			}
		}
		dir, err := GetUnwrap(ctx, storage, path)
		if err != nil {
			return nil, errors.WithMessage(err, "failed get dir")
		}
		if !dir.IsDir() {
			return nil, errors.WithStack(errs.NotFolder)
		}
		page, err := reader.ListPage(ctx, dir, args)
		if err != nil {
			return nil, errors.WithMessage(err, "failed to list page")
		}
		if page == nil || page.Total < 0 || int64(len(page.Objs)) > args.Limit {
			return nil, errors.New("invalid page result")
		}
		count := int64(len(page.Objs))
		if (args.Offset < page.Total && count > page.Total-args.Offset) || (args.Offset >= page.Total && count != 0) {
			return nil, errors.New("page result exceeds total")
		}
		page = clonePage(page)
		for _, obj := range page.Objs {
			if obj == nil {
				return nil, errors.New("nil object in page result")
			}
			if setter, ok := obj.(model.SetPath); ok && obj.GetPath() == "" && dir.GetPath() != "" {
				setter.SetPath(stdpath.Join(dir.GetPath(), obj.GetName()))
			}
		}
		model.WrapObjsName(page.Objs)
		if useCache {
			pageMu.Lock()
			if pageGenerations[storage.GetStorage()] == generation {
				pageCache.Set(key, page, cache.WithEx[*model.PageList](time.Minute*time.Duration(storage.GetStorage().CacheExpiration)))
			}
			pageMu.Unlock()
		}
		return page, nil
	})
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case res := <-result:
		return clonePage(res.Val), res.Err
	}
}
