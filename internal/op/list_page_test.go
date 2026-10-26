package op

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

type pageTestDriver struct {
	model.Storage
	mu     sync.Mutex
	ranges []model.PageListArgs
	full   atomic.Int32
	getErr error
	fetch  func(context.Context, model.PageListArgs) (*model.PageList, error)
}

func (d *pageTestDriver) Config() driver.Config          { return driver.Config{LocalSort: true} }
func (d *pageTestDriver) GetAddition() driver.Additional { return &struct{}{} }
func (d *pageTestDriver) Init(context.Context) error     { return nil }
func (d *pageTestDriver) Drop(context.Context) error     { return nil }
func (d *pageTestDriver) Link(context.Context, model.Obj, model.LinkArgs) (*model.Link, error) {
	return nil, errs.NotImplement
}
func (d *pageTestDriver) List(context.Context, model.Obj, model.ListArgs) ([]model.Obj, error) {
	d.full.Add(1)
	return []model.Obj{&model.Object{Name: "full"}}, nil
}
func (d *pageTestDriver) Get(_ context.Context, path string) (model.Obj, error) {
	if path == "/" {
		return nil, errs.NotImplement
	}
	if d.getErr != nil {
		return nil, d.getErr
	}
	return &model.Object{ID: path, Path: path, IsFolder: true}, nil
}
func (d *pageTestDriver) GetRoot(context.Context) (model.Obj, error) {
	return &model.Object{ID: "root", Path: "/root", IsFolder: true}, nil
}
func (d *pageTestDriver) ListPage(ctx context.Context, _ model.Obj, args model.PageListArgs) (*model.PageList, error) {
	d.mu.Lock()
	d.ranges = append(d.ranges, args)
	d.mu.Unlock()
	if d.fetch != nil {
		return d.fetch(ctx, args)
	}
	return &model.PageList{Objs: []model.Obj{&model.Object{Name: "z"}, &model.Object{Name: "a"}}, Total: 10, HasMore: true}, nil
}
func (d *pageTestDriver) calls() int { d.mu.Lock(); defer d.mu.Unlock(); return len(d.ranges) }
func newPageTestDriver(t *testing.T) *pageTestDriver {
	d := &pageTestDriver{Storage: model.Storage{MountPath: "/" + t.Name(), CacheExpiration: 1}}
	d.OrderBy = "name"
	return d
}
func pageArgs() model.PageListArgs { return model.PageListArgs{Limit: 2} }
func mustPage(t *testing.T, d *pageTestDriver, path string, a model.PageListArgs) *model.PageList {
	t.Helper()
	p, e := ListPage(context.Background(), d, path, a)
	if e != nil {
		t.Fatal(e)
	}
	return p
}

func TestListPageBoundedCacheAndCopies(t *testing.T) {
	d := newPageTestDriver(t)
	a := pageArgs()
	p := mustPage(t, d, "/", a)
	if p.Objs[0].GetName() != "z" || p.Objs[0].GetPath() != "/root/z" {
		t.Fatalf("page sorted or path missing: %+v", p.Objs)
	}
	model.UnwrapObj(p.Objs[0]).(*model.Object).SetPath("/mutated")
	p.Objs[0] = nil
	p.Total = 0
	if p = mustPage(t, d, "/", a); p.Objs[0] == nil || p.Total != 10 || p.Objs[0].GetPath() != "/root/z" {
		t.Fatal("caller mutated cached page")
	}
	a.Offset = 2
	mustPage(t, d, "/", a)
	if d.calls() != 2 || d.ranges[1].Offset != 2 || d.ranges[1].Limit != 2 {
		t.Fatalf("unexpected ranges: %+v", d.ranges)
	}
	if _, ok := listCache.Get(Key(d, "/")); ok {
		t.Fatal("page entered full cache")
	}
	if _, e := List(context.Background(), d, "/", model.ListArgs{NoUpdateIndex: true}); e != nil {
		t.Fatal(e)
	}
	mustPage(t, d, "/", a)
	if d.full.Load() != 1 || d.calls() != 2 {
		t.Fatal("page and full caches not separate")
	}
	a.OrderBy = "name"
	mustPage(t, d, "/", a)
	if d.calls() != 3 {
		t.Fatal("sort missing from cache key")
	}
	other := newPageTestDriver(t)
	mustPage(t, other, "/", a)
	if other.calls() != 1 {
		t.Fatal("instance inherited cache")
	}
}
func TestListPageInvalidation(t *testing.T) {
	d := newPageTestDriver(t)
	a := pageArgs()
	old := &model.Object{Name: "old", IsFolder: true}
	next := &model.Object{Name: "new", IsFolder: true}
	mutations := []func(){func() { updateCacheObj(d, "/", old, next) }, func() { delCacheObj(d, "/", old) }, func() { addCacheObj(d, "/", next) }, func() { ClearCache(d, "/") }}
	for _, mutate := range mutations {
		mustPage(t, d, "/child/grandchild", a)
		before := d.calls()
		mutate()
		mustPage(t, d, "/child/grandchild", a)
		if d.calls() != before+1 {
			t.Fatal("descendant page survived mutation")
		}
	}
	a.Offset = 2
	mustPage(t, d, "/", a)
	before := d.calls()
	refresh := pageArgs()
	refresh.Refresh = true
	mustPage(t, d, "/", refresh)
	if d.calls() != before+1 {
		t.Fatal("refresh fetched extra pages")
	}
	mustPage(t, d, "/", a)
	if d.calls() != before+2 {
		t.Fatal("refresh left other page cached")
	}
}
func TestListPageUnsupported(t *testing.T) {
	d := newPageTestDriver(t)
	plain := struct{ driver.Driver }{Driver: d}
	if _, err := ListPage(context.Background(), plain, "/", pageArgs()); !errors.Is(err, errs.NotImplement) {
		t.Fatalf("non-paged driver error = %v", err)
	}
}

func TestListPageFullRefreshInvalidates(t *testing.T) {
	d := newPageTestDriver(t)
	mustPage(t, d, "/", pageArgs())
	if _, err := List(context.Background(), d, "/", model.ListArgs{Refresh: true, NoUpdateIndex: true}); err != nil {
		t.Fatal(err)
	}
	mustPage(t, d, "/", pageArgs())
	if d.calls() != 2 {
		t.Fatal("full-list refresh left page cached")
	}
}

func TestListPageNoCacheAndValidation(t *testing.T) {
	d := newPageTestDriver(t)
	d.CacheExpiration = 0
	a := pageArgs()
	mustPage(t, d, "/", a)
	mustPage(t, d, "/", a)
	if d.calls() != 2 {
		t.Fatal("zero expiration cached")
	}
	for _, args := range []model.PageListArgs{{Limit: 0}, {Offset: -1, Limit: 2}} {
		if _, e := ListPage(context.Background(), d, "/", args); e == nil {
			t.Fatal("invalid range accepted")
		}
	}
	for _, p := range []*model.PageList{nil, {Total: -1}, {Objs: []model.Obj{nil}, Total: 1}, {Objs: []model.Obj{&model.Object{}}, Total: 0}, {Objs: make([]model.Obj, 3), Total: 10}} {
		d.fetch = func(context.Context, model.PageListArgs) (*model.PageList, error) { return p, nil }
		if _, e := ListPage(context.Background(), d, "/", a); e == nil {
			t.Fatalf("invalid result accepted: %+v", p)
		}
	}
}
func TestPagedGetterErrorAuthoritative(t *testing.T) {
	d := newPageTestDriver(t)
	d.getErr = errors.New("upstream failed")
	if _, e := Get(context.Background(), d, "/child"); !errors.Is(e, d.getErr) {
		t.Fatalf("get error = %v", e)
	}
	if d.full.Load() != 0 {
		t.Fatal("Getter error fell back to full List")
	}
	if _, e := Get(context.Background(), d, "/"); e != nil {
		t.Fatal("root NotImplement did not resolve:", e)
	}
}
func TestListPageCancellationAndInflightInvalidation(t *testing.T) {
	d := newPageTestDriver(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	d.fetch = func(ctx context.Context, a model.PageListArgs) (*model.PageList, error) {
		once.Do(func() { close(started) })
		select {
		case <-release:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		return &model.PageList{Total: 0}, nil
	}
	done := make(chan error, 1)
	go func() { _, e := ListPage(context.Background(), d, "/", pageArgs()); done <- e }()
	<-started
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, e := ListPage(ctx, d, "/", pageArgs()); !errors.Is(e, context.DeadlineExceeded) {
		t.Fatalf("waiting cancellation=%v", e)
	}
	if d.calls() != 1 {
		t.Fatal("same-page requests not shared")
	}
	ClearCache(d, "/")
	close(release)
	if e := <-done; e != nil {
		t.Fatal(e)
	}
	mustPage(t, d, "/", pageArgs())
	if d.calls() != 2 {
		t.Fatal("stale in-flight cache write survived invalidation")
	}
}
func TestListPageNoPartialHook(t *testing.T) {
	old := objsUpdateHooks
	var called atomic.Int32
	objsUpdateHooks = []ObjsUpdateHook{func(string, []model.Obj) { called.Add(1) }}
	defer func() { objsUpdateHooks = old }()
	mustPage(t, newPageTestDriver(t), "/", pageArgs())
	time.Sleep(20 * time.Millisecond)
	if called.Load() != 0 {
		t.Fatal("partial listing invoked full-directory hook")
	}
}
