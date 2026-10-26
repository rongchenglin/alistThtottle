package fs

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/db"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
	"github.com/alist-org/alist/v3/internal/op"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

type fsPageDriver struct {
	model.Storage
	calls atomic.Int32
}

func (d *fsPageDriver) Config() driver.Config          { return driver.Config{Name: "FsPageTest"} }
func (d *fsPageDriver) GetAddition() driver.Additional { return &struct{}{} }
func (d *fsPageDriver) Init(context.Context) error     { return nil }
func (d *fsPageDriver) Drop(context.Context) error     { return nil }
func (d *fsPageDriver) GetRoot(context.Context) (model.Obj, error) {
	return &model.Object{IsFolder: true}, nil
}
func (d *fsPageDriver) List(context.Context, model.Obj, model.ListArgs) ([]model.Obj, error) {
	panic("unexpected full List")
}
func (d *fsPageDriver) Link(context.Context, model.Obj, model.LinkArgs) (*model.Link, error) {
	return nil, errs.NotImplement
}
func (d *fsPageDriver) ListPage(_ context.Context, _ model.Obj, a model.PageListArgs) (*model.PageList, error) {
	d.calls.Add(1)
	return &model.PageList{Objs: []model.Obj{&model.Object{Name: "hidden"}, &model.Object{Name: "visible"}, &model.Object{Name: "visible"}}, Total: 20, HasMore: true}, nil
}

type fsPlainDriver struct{ *fsPageDriver }

func (d *fsPlainDriver) Config() driver.Config { return driver.Config{Name: "FsPlainTest"} }

// Hide the embedded optional interface by exposing only the required Driver methods.
type plainOnly struct{ driver.Driver }

func TestListPageFilteringAndVirtualChildren(t *testing.T) {
	conf.Conf = conf.DefaultConfig()
	database, err := gorm.Open(sqlite.Open(filepath.Join(t.TempDir(), "page.db")), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	db.Init(database)
	sqlDB, err := database.DB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := sqlDB.Close(); err != nil {
			t.Error(err)
		}
	})
	op.RegisterDriver(func() driver.Driver { return &fsPageDriver{} })
	op.RegisterDriver(func() driver.Driver { return &plainOnly{Driver: &fsPlainDriver{fsPageDriver: &fsPageDriver{}}} })
	ctx := context.Background()
	const mount = "/fs-page-test"
	id, err := op.CreateStorage(ctx, model.Storage{Driver: "FsPageTest", MountPath: mount, Addition: "{}", CacheExpiration: 1})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := op.DeleteStorageById(ctx, id); err != nil {
			t.Error(err)
		}
	})
	storage, err := op.GetStorageByMountPath(mount)
	if err != nil {
		t.Fatal(err)
	}
	fake := storage.(*fsPageDriver)
	ctx = context.WithValue(ctx, "user", &model.User{})
	ctx = context.WithValue(ctx, "meta", &model.Meta{Path: mount, Hide: "^hidden$"})
	page, err := ListPage(ctx, mount, model.PageListArgs{Limit: 3})
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Objs) != 1 || page.Objs[0].GetName() != "visible" || page.Total != 20 || !page.HasMore {
		t.Fatalf("filtered page=%+v", page)
	}
	if fake.calls.Load() != 1 {
		t.Fatal("filtering refilled page")
	}
	page, err = ListPage(context.Background(), mount, model.PageListArgs{Limit: 3})
	if err != nil || len(page.Objs) != 2 {
		t.Fatalf("filtering contaminated cache: %+v %v", page, err)
	}
	childID, err := op.CreateStorage(context.Background(), model.Storage{Driver: "FsPageTest", MountPath: mount + "/child", Addition: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := op.DeleteStorageById(context.Background(), childID); err != nil {
			t.Error(err)
		}
	}()
	if _, err = ListPage(ctx, mount, model.PageListArgs{Limit: 3}); !errors.Is(err, errs.NotSupport) {
		t.Fatalf("virtual children error=%v", err)
	}
	if fake.calls.Load() != 1 {
		t.Fatal("virtual children fetched provider page")
	}
	plainID, err := op.CreateStorage(context.Background(), model.Storage{Driver: "FsPlainTest", MountPath: "/fs-plain-test", Addition: "{}"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := op.DeleteStorageById(context.Background(), plainID); err != nil {
			t.Error(err)
		}
	}()
	if _, err = ListPage(ctx, "/fs-plain-test", model.PageListArgs{Limit: 3}); !errors.Is(err, errs.NotImplement) {
		t.Fatalf("plain driver error=%v", err)
	}
}
