package handles

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type sourcePageTestDriver struct {
	model.Storage
	ranges []model.PageListArgs
}

func (d *sourcePageTestDriver) Config() driver.Config {
	return driver.Config{Name: "SourcePageHandlerTest"}
}
func (d *sourcePageTestDriver) GetAddition() driver.Additional { return &struct{}{} }
func (d *sourcePageTestDriver) Init(context.Context) error     { return nil }
func (d *sourcePageTestDriver) Drop(context.Context) error     { return nil }
func (d *sourcePageTestDriver) Get(_ context.Context, path string) (model.Obj, error) {
	return &model.Object{ID: path, Path: path, Name: "directory", IsFolder: true}, nil
}
func (d *sourcePageTestDriver) List(context.Context, model.Obj, model.ListArgs) ([]model.Obj, error) {
	panic("web browsing must never request a complete directory")
}
func (d *sourcePageTestDriver) Link(context.Context, model.Obj, model.LinkArgs) (*model.Link, error) {
	return nil, errs.NotImplement
}
func (d *sourcePageTestDriver) ListPage(_ context.Context, _ model.Obj, args model.PageListArgs) (*model.PageList, error) {
	d.ranges = append(d.ranges, args)
	return &model.PageList{Objs: []model.Obj{&model.Object{Name: "hidden.txt"}}, Total: 10000, HasMore: true}, nil
}

func TestFsListUsesOnlyRequestedUpstreamPage(t *testing.T) {
	oldConf := conf.Conf
	conf.Conf = conf.DefaultConfig()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "pages.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.Init(database)
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		op.ClearUserCache()
		op.SettingCacheUpdate()
		conf.Conf = oldConf
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	const mount = "/source-page-handler-test"
	op.RegisterDriver(func() driver.Driver { return &sourcePageTestDriver{} })
	id, err := op.CreateStorage(context.Background(), model.Storage{Driver: "SourcePageHandlerTest", MountPath: mount, Addition: "{}", CacheExpiration: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		for _, roleID := range []uint{4021, 4022} {
			if err := db.DeleteRole(roleID); err != nil {
				t.Error(err)
			}
		}
		if err := op.DeleteStorageById(context.Background(), id); err != nil {
			t.Error(err)
		}
	})
	storage, err := op.GetStorageByMountPath(mount)
	if err != nil {
		t.Fatal(err)
	}
	fake := storage.(*sourcePageTestDriver)
	role := model.Role{ID: 4021, Name: "page-reader", PermissionScopes: []model.PermissionEntry{{Path: mount, Permission: 0}}}
	if err := db.CreateRole(&role); err != nil {
		t.Fatal(err)
	}
	meta := model.Meta{Path: mount, Hide: "^hidden", HSub: true}
	if err := op.CreateMeta(&meta); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := op.DeleteMetaById(meta.ID); err != nil {
			t.Error(err)
		}
	})
	user := &model.User{BasePath: "/", Role: model.Roles{4021}}
	invoke := func(handler gin.HandlerFunc, endpoint string, payload any) []byte {
		t.Helper()
		body, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user", user)
		handler(c)
		var response struct {
			Code    int
			Message string
		}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil || response.Code != 200 {
			t.Fatalf("handler failed: %s, %v", w.Body.String(), err)
		}
		return w.Body.Bytes()
	}
	readPage := func(page, size int) FsListResp {
		t.Helper()
		raw := invoke(FsList, "/api/fs/list", map[string]any{"path": mount, "page": page, "per_page": size})
		var response struct{ Data FsListResp }
		if err := json.Unmarshal(raw, &response); err != nil {
			t.Fatal(err)
		}
		return response.Data
	}
	first := readPage(1, -1)
	if !first.SourcePagination || first.PerPage != 50 || first.TotalExact || !first.HasMore || len(first.Content) != 0 {
		t.Fatalf("all request or hide handling is wrong: %+v", first)
	}
	if len(fake.ranges) != 1 || fake.ranges[0].Offset != 0 || fake.ranges[0].Limit != 50 {
		t.Fatalf("unexpected requests: %+v", fake.ranges)
	}
	second := readPage(2, 50)
	if second.Page != 2 || !second.HasMore || len(fake.ranges) != 2 || fake.ranges[1].Offset != 50 {
		t.Fatalf("second page fetched extra ranges: %+v", fake.ranges)
	}
	readPage(1, 50)
	if len(fake.ranges) != 2 {
		t.Fatal("cached page was fetched again")
	}
	// A cold nested-directory fs/get must not fetch a parent for related files.
	raw := invoke(FsGet, "/api/fs/get", map[string]any{"path": mount + "/deep/child"})
	var detail struct{ Data FsGetResp }
	if err := json.Unmarshal(raw, &detail); err != nil {
		t.Fatal(err)
	}
	if !detail.Data.SourcePagination || !detail.Data.IsDir || len(fake.ranges) != 2 {
		t.Fatal("directory detail requested another list")
	}
	// Unreadable children stay filtered without refilling another upstream page.
	restrictedRole := model.Role{ID: 4022, Name: "narrow-reader", PermissionScopes: []model.PermissionEntry{{Path: mount + "/allowed", Permission: 0}}}
	if err := db.CreateRole(&restrictedRole); err != nil {
		t.Fatal(err)
	}
	user.Role = model.Roles{4022}
	filtered := readPage(2, 50)
	if len(filtered.Content) != 0 || !filtered.HasMore || len(fake.ranges) != 2 {
		t.Fatal("permissions leaked objects or triggered a refill")
	}
}

func TestSourcePageArgsValidateBoundsAndSort(t *testing.T) {
	args, err := sourcePageArgs(ListReq{PageReq: model.PageReq{Page: 3, PerPage: 50}, OrderBy: "name", OrderDirection: "desc"})
	if err != nil || args.Offset != 100 || args.Limit != 50 || args.OrderBy != "name" || args.OrderDirection != "desc" {
		t.Fatalf("bad range/sort: %+v %v", args, err)
	}
	for _, req := range []ListReq{
		{PageReq: model.PageReq{Page: 1, PerPage: -1}},
		{PageReq: model.PageReq{Page: model.MaxInt, PerPage: 500}},
		{PageReq: model.PageReq{Page: 1, PerPage: 50}, OrderBy: "bad"},
		{PageReq: model.PageReq{Page: 1, PerPage: 50}, OrderDirection: "bad"},
	} {
		if _, err := sourcePageArgs(req); err == nil {
			t.Fatalf("accepted invalid request: %+v", req)
		}
	}
}
