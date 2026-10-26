package _115

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"

	driver115 "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/alist-org/alist/v3/internal/conf"
	"github.com/alist-org/alist/v3/internal/driver"
	"github.com/alist-org/alist/v3/internal/errs"
	"github.com/alist-org/alist/v3/internal/model"
)

func TestListPageRequestsOnlyRequestedRange(t *testing.T) {
	var ranges []string
	d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/files" {
			t.Fatalf("unexpected endpoint: %s", req.URL.Path)
		}
		q := req.URL.Query()
		ranges = append(ranges, q.Get("offset")+":"+q.Get("limit"))
		if q.Get("o") != "file_name" || q.Get("asc") != "0" {
			t.Errorf("sort was not passed upstream: %s", req.URL.RawQuery)
		}
		var entries []string
		for i := 0; i < 50; i++ {
			entries = append(entries, fmt.Sprintf(`{"fid":"%d","cid":"0","n":"file%d"}`, i+1, i+1))
		}
		return testResponse(req, 200, fmt.Sprintf(`{"state":true,"cid":"0","count":10000,"offset":%s,"data":[%s]}`, q.Get("offset"), strings.Join(entries, ","))), nil
	}))
	for _, offset := range []int64{0, 50} {
		result, err := d.ListPage(context.Background(), &FileObj{}, model.PageListArgs{Offset: offset, Limit: 50, OrderBy: "name", OrderDirection: "desc"})
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Objs) != 50 || result.Total != 10000 || !result.HasMore {
			t.Fatalf("unexpected page: %+v", result)
		}
	}
	if strings.Join(ranges, ",") != "0:50,50:50" {
		t.Fatalf("fetched unrequested pages: %v", ranges)
	}
}

func TestListPageDoesNotFillShortPage(t *testing.T) {
	calls := 0
	d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) { calls++; return testPage(req, 10000), nil }))
	result, err := d.ListPage(context.Background(), &FileObj{}, model.PageListArgs{Offset: 0, Limit: 50})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 1 || len(result.Objs) != 1 || !result.HasMore {
		t.Fatalf("short page caused refill: calls=%d result=%+v", calls, result)
	}
}

func TestListPageRejectsUnboundedOrInvalidRequest(t *testing.T) {
	d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatal("invalid request reached upstream")
		return nil, nil
	}))
	for _, args := range []model.PageListArgs{{Limit: -1}, {Limit: 0}, {Offset: -1, Limit: 50}, {Limit: 1151}, {Limit: 50, OrderBy: "bad"}, {Limit: 50, OrderDirection: "bad"}} {
		if _, err := d.ListPage(context.Background(), &FileObj{}, args); err == nil {
			t.Fatalf("accepted invalid page: %+v", args)
		}
	}
}

func TestGetDirectoryUsesPathLookupNotParentList(t *testing.T) {
	calls := 0
	d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		if req.URL.String() != driver115.ApiDirName2CID+"?path=%2Flarge%2Fchild" {
			t.Fatalf("directory discovery scanned instead of direct lookup: %s", req.URL)
		}
		return testResponse(req, 200, `{"state":true,"id":"12345"}`), nil
	}))
	obj, err := d.Get(context.Background(), "/large/child")
	if err != nil {
		t.Fatal(err)
	}
	if !obj.IsDir() || obj.GetID() != "12345" || obj.GetName() != "child" || calls != 1 {
		t.Fatalf("unexpected directory: %+v, calls=%d", obj, calls)
	}
}

func TestGetDirectoryUnderNonRootMount(t *testing.T) {
	var calls []string
	d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls = append(calls, req.URL.Path)
		switch req.URL.Path {
		case "/category/get":
			return testResponse(req, 200, `{"file_name":"mount","paths":[{"file_id":0,"file_name":"root"},{"file_id":42,"file_name":"mount"}]}`), nil
		case "/files/getid":
			if req.URL.Query().Get("path") != "/mount/child" {
				t.Fatalf("wrong mounted path: %s", req.URL)
			}
			return testResponse(req, 200, `{"state":true,"id":"99"}`), nil
		default:
			t.Fatalf("unexpected full scan: %s", req.URL)
			return nil, nil
		}
	}))
	d.RootID = driver.RootID{RootFolderID: "42"}
	obj, err := d.Get(context.Background(), "/child")
	if err != nil || obj.GetID() != "99" || strings.Join(calls, ",") != "/category/get,/files/getid" {
		t.Fatalf("obj=%v err=%v calls=%v", obj, err, calls)
	}
}

func TestPageObjectCacheInvalidationAndCopies(t *testing.T) {
	d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		t.Fatalf("cached Get sent HTTP request: %s", req.URL)
		return nil, nil
	}))
	d.CacheExpiration = 1
	file := &FileObj{File: driver115.File{FileID: "1", Name: "file"}, Path: "/file"}
	d.rememberPageObjects("/", []model.Obj{file}, 0)
	file.Name = "changed"
	obj, err := d.Get(context.Background(), "/file")
	if err != nil || obj.GetName() != "file" {
		t.Fatalf("cache changed with source: %v %v", obj, err)
	}
	obj.(*FileObj).Name = "changed again"
	obj, err = d.Get(context.Background(), "/file")
	if err != nil || obj.GetName() != "file" {
		t.Fatal("Get exposed cached object")
	}
	d.InvalidatePageObjects()
	d.rememberPageObjects("/", []model.Obj{file}, 0)
	if _, ok := d.objectCache().Get("/file"); ok {
		t.Fatal("stale in-flight page repopulated object cache")
	}
}

func TestColdMappedPathDoesNotGuessOrScan(t *testing.T) {
	old := conf.FilenameCharMap
	conf.FilenameCharMap = map[string]string{":": "_"}
	defer func() { conf.FilenameCharMap = old }()
	d := &Pan115{}
	if _, err := d.nativeRelativePath("/a_b/child"); !errors.Is(err, errs.NotSupport) {
		t.Fatalf("mapped cold path guessed: %v", err)
	}
	d.CacheExpiration = 1
	d.rememberPageObjects("/", []model.Obj{&FileObj{File: driver115.File{FileID: "1", Name: "a:b", IsDirectory: true}}}, 0)
	if native, err := d.nativeRelativePath("/a_b/child"); err != nil || native != "a:b/child" {
		t.Fatalf("native=%q err=%v", native, err)
	}
}

func TestColdAmbiguousFileLookupStopsWithoutFullScan(t *testing.T) {
	calls := 0
	d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls++
		switch req.URL.Path {
		case "/files/getid":
			return testResponse(req, 200, `{"state":true,"id":"0"}`), nil
		case "/files/search":
			if req.URL.Query().Get("limit") != "50" || req.URL.Query().Get("offset") != "0" {
				t.Fatalf("unbounded search: %s", req.URL)
			}
			return testResponse(req, 200, `{"state":true,"count":1000,"data":[]}`), nil
		default:
			t.Fatalf("full scan: %s", req.URL)
			return nil, nil
		}
	}))
	if _, err := d.Get(context.Background(), "/file.txt"); !errors.Is(err, errs.NotSupport) {
		t.Fatalf("ambiguous lookup returned %v", err)
	}
	if calls != 2 {
		t.Fatalf("unexpected request count: %d", calls)
	}
}
