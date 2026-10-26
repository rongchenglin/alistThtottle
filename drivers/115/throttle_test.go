package _115

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	driver115 "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/alist-org/alist/v3/internal/model"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func testResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     http.Header{"Content-Type": {"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
		Request:    req,
	}
}

func testPage(req *http.Request, count int) *http.Response {
	offset, _ := strconv.Atoi(req.URL.Query().Get("offset"))
	return testResponse(req, http.StatusOK, fmt.Sprintf(
		`{"state":true,"cid":"0","count":%d,"offset":%d,"data":[{"fid":"%d","cid":"0","n":"file%d"}]}`,
		count, offset, offset+1, offset+1))
}

func testPan115(rps float64, transport http.RoundTripper) *Pan115 {
	d := &Pan115{Addition: Addition{PageSize: 1, LimitRate: rps}}
	d.client = driver115.New(driver115.WithClient(&http.Client{Transport: transport}))
	d.configureClient()
	return d
}

func TestListThrottlesEveryPage(t *testing.T) {
	var mu sync.Mutex
	var sent []time.Time
	d := testPan115(10, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		sent = append(sent, time.Now())
		mu.Unlock()
		return testPage(req, 3), nil
	}))
	files, err := d.List(context.Background(), &FileObj{}, model.ListArgs{})
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 3 || len(sent) != 3 {
		t.Fatalf("got %d files and %d requests, want 3 each", len(files), len(sent))
	}
	for i := 1; i < len(sent); i++ {
		if gap := sent[i].Sub(sent[i-1]); gap < 80*time.Millisecond {
			t.Errorf("page %d followed previous page after %s; limiter was bypassed", i+1, gap)
		}
	}
}

func TestConcurrentListsShareLimiter(t *testing.T) {
	var mu sync.Mutex
	var sent []time.Time
	d := testPan115(10, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		mu.Lock()
		sent = append(sent, time.Now())
		mu.Unlock()
		return testPage(req, 1), nil
	}))
	var wg sync.WaitGroup
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := d.List(context.Background(), &FileObj{}, model.ListArgs{}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	if len(sent) != 3 {
		t.Fatalf("got %d requests, want 3", len(sent))
	}
	sort.Slice(sent, func(i, j int) bool { return sent[i].Before(sent[j]) })
	if elapsed := sent[2].Sub(sent[0]); elapsed < 180*time.Millisecond {
		t.Fatalf("concurrent lists bypassed shared limiter: %s", elapsed)
	}
}

func TestNonPositiveRateUsesDefault(t *testing.T) {
	for _, rps := range []float64{0, -1} {
		d := testPan115(rps, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return testPage(req, 1), nil
		}))
		if d.limiter == nil || d.limiter.Limit() != 0.5 || d.limiter.Burst() != 1 {
			t.Fatalf("rate %v did not enable default 0.5 requests/second, burst 1", rps)
		}
	}
}

func TestListCancelsWhileWaitingForNextPage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var calls atomic.Int32
	d := testPan115(0.5, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		cancel()
		return testPage(req, 3), nil
	}))
	files, err := d.List(ctx, &FileObj{}, model.ListArgs{})
	if !errors.Is(err, context.Canceled) || files != nil || calls.Load() != 1 {
		t.Fatalf("got files=%v, err=%v, requests=%d; want cancellation after one page", files, err, calls.Load())
	}
}

func TestListStopsAndCoolsDownOnThrottle(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
	}{
		{"http429", 429, `{"state":false}`},
		{"http405", 405, `<html>blocked</html>`},
		{"riskControl", 200, `{"state":false,"errno":911,"error":"verification required"}`},
		{"numericState", 200, `{"state":1,"code":911}`},
		{"missingState", 200, `{"errno":"429"}`},
		{"alternateErrno", 200, `{"state":false,"errNo":911}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) {
				if calls.Add(1) == 1 {
					return testPage(req, 3), nil
				}
				resp := testResponse(req, tc.status, tc.body)
				resp.Header.Set("Retry-After", "600")
				return resp, nil
			}))
			files, err := d.List(context.Background(), &FileObj{}, model.ListArgs{})
			if !errors.Is(err, errRequestLimited) || files != nil || calls.Load() != 2 {
				t.Fatalf("got files=%v err=%v requests=%d; want throttle after page 2 without partial results", files, err, calls.Load())
			}
			_, err = d.List(context.Background(), &FileObj{}, model.ListArgs{})
			if !errors.Is(err, errRequestLimited) || calls.Load() != 2 {
				t.Fatalf("cooldown sent another request: err=%v requests=%d", err, calls.Load())
			}
		})
	}
}

func TestCooldownHonorsRetryAfter(t *testing.T) {
	for _, value := range []string{"", "bad", "-1", "1", "600", time.Now().Add(15 * time.Minute).UTC().Format(http.TimeFormat)} {
		t.Run(value, func(t *testing.T) {
			d := &Pan115{}
			before := time.Now()
			if err := d.startCooldown(value); !errors.Is(err, errRequestLimited) {
				t.Fatalf("got %v, want rate limit error", err)
			}
			minimum := before.Add(5 * time.Minute)
			if value == "600" {
				minimum = before.Add(10 * time.Minute)
			} else if date, err := http.ParseTime(value); err == nil {
				minimum = date
			}
			if d.cooldownUntil.Before(minimum) {
				t.Fatalf("cooldown ends at %v, before required %v", d.cooldownUntil, minimum)
			}
			previous := d.cooldownUntil
			_ = d.startCooldown("1")
			if d.cooldownUntil.Before(previous) {
				t.Fatal("another response shortened the cooldown")
			}
		})
	}
}

func TestQueuedRequestChecksCooldownAgain(t *testing.T) {
	var calls atomic.Int32
	d := testPan115(0.5, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return testPage(req, 1), nil
	}))
	if _, err := d.List(context.Background(), &FileObj{}, model.ListArgs{}); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() {
		_, err := d.List(context.Background(), &FileObj{}, model.ListArgs{})
		done <- err
	}()
	// Wait until the second request has reserved a future token.
	deadline := time.Now().Add(time.Second)
	for d.limiter.Tokens() > -0.5 {
		if time.Now().After(deadline) {
			t.Fatal("second request did not enter the limiter queue")
		}
		time.Sleep(time.Millisecond)
	}
	_ = d.startCooldown("")
	select {
	case err := <-done:
		if !errors.Is(err, errRequestLimited) || calls.Load() != 1 {
			t.Fatalf("queued request ignored cooldown: err=%v calls=%d", err, calls.Load())
		}
	case <-time.After(250 * time.Millisecond):
		t.Fatal("cooldown did not wake queued request promptly")
	}
}

func TestDecodedUploadThrottleResponse(t *testing.T) {
	d := &Pan115{}
	if err := d.checkThrottleResponse([]byte(`{"statuscode":911,"status":0}`), ""); !errors.Is(err, errRequestLimited) {
		t.Fatalf("decoded upload rejection did not trigger cooldown: %v", err)
	}
	if err := d.checkRequest(context.Background()); !errors.Is(err, errRequestLimited) {
		t.Fatalf("request not blocked after upload rejection: %v", err)
	}
}

func TestNormalResponseDoesNotTriggerCooldown(t *testing.T) {
	for _, body := range []string{`{"state":true,"data":"encrypted"}`, `{"state":true,"code":911}`, `{"state":false,"errno":99}`, `{"statuscode":701,"status":1}`, "binary response"} {
		d := &Pan115{}
		if err := d.checkThrottleResponse([]byte(body), ""); err != nil {
			t.Fatalf("normal response %q triggered cooldown: %v", body, err)
		}
	}
}

func TestListRejectsInvalidPages(t *testing.T) {
	for _, body := range []string{
		`{"state":true,"cid":"0","count":10,"offset":0,"data":[]}`,
		`{"state":true,"cid":"0","count":10,"offset":9,"data":[{"fid":"1","n":"file"}]}`,
	} {
		var calls atomic.Int32
		d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) {
			if calls.Add(1) > 1 {
				return nil, fmt.Errorf("unexpected second request")
			}
			return testResponse(req, 200, body), nil
		}))
		files, err := d.List(context.Background(), &FileObj{}, model.ListArgs{})
		if !errors.Is(err, driver115.ErrUnexpected) || files != nil || calls.Load() != 1 {
			t.Fatalf("invalid page not stopped: files=%v err=%v requests=%d", files, err, calls.Load())
		}
	}
}

func TestListUsesActualPageLength(t *testing.T) {
	var offsets []string
	d := testPan115(1000, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		offsets = append(offsets, req.URL.Query().Get("offset"))
		return testPage(req, 3), nil
	}))
	d.PageSize = 50
	files, err := d.List(context.Background(), &FileObj{}, model.ListArgs{})
	if err != nil || len(files) != 3 || strings.Join(offsets, ",") != "0,1,2" {
		t.Fatalf("short page skipped files: files=%d offsets=%v err=%v", len(files), offsets, err)
	}
}

func TestListDeadlineStopsLimiterWait(t *testing.T) {
	var calls atomic.Int32
	d := testPan115(0.5, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		calls.Add(1)
		return testPage(req, 3), nil
	}))
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	files, err := d.List(ctx, &FileObj{}, model.ListArgs{})
	if err == nil || files != nil || calls.Load() != 1 {
		t.Fatalf("deadline not respected: files=%v err=%v requests=%d", files, err, calls.Load())
	}
}

func TestListCancelsInFlightPage(t *testing.T) {
	started := make(chan struct{})
	release := make(chan struct{})
	defer close(release)
	d := testPan115(100, roundTripFunc(func(req *http.Request) (*http.Response, error) {
		close(started)
		select {
		case <-req.Context().Done():
			return nil, req.Context().Err()
		case <-release:
			return testPage(req, 1), nil
		}
	}))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := d.List(ctx, &FileObj{}, model.ListArgs{})
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("cancelled list returned success")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled list kept its HTTP request running")
	}
}
