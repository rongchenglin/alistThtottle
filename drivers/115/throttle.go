package _115

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"time"

	driver115 "github.com/SheltonZhu/115driver/pkg/driver"
	"github.com/go-resty/resty/v2"
	"github.com/pkg/errors"
	"golang.org/x/time/rate"
)

const (
	defaultLimitRate = 0.5
	defaultCooldown  = 5 * time.Minute
)

var errRequestLimited = errors.New("115 API rate limited or risk control triggered")

// configureClient must run before the client is used, including login requests.
func (d *Pan115) configureClient() {
	limit := d.LimitRate
	if limit <= 0 || math.IsNaN(limit) || math.IsInf(limit, 0) {
		limit = defaultLimitRate
	}
	d.limiter = rate.NewLimiter(rate.Limit(limit), 1)
	transport := d.client.Client.GetClient().Transport
	if transport == nil {
		transport = http.DefaultTransport
	}
	d.client.Client.SetTransport(&limitedTransport{driver: d, base: transport}).
		SetRetryCount(0).
		SetTimeout(30 * time.Second).
		OnAfterResponse(func(_ *resty.Client, resp *resty.Response) error {
			return d.checkThrottleResponse(resp.Body(), resp.Header().Get("Retry-After"))
		})
}

func (d *Pan115) checkThrottleResponse(body []byte, retryAfter string) error {
	var result map[string]json.RawMessage
	// Encrypted and non-JSON responses are checked by their callers after decoding.
	if json.Unmarshal(body, &result) != nil || string(result["state"]) == "true" {
		return nil
	}
	for _, key := range []string{"errno", "errNo", "code", "statuscode"} {
		var code driver115.StringInt
		if json.Unmarshal(result[key], &code) == nil && (code == 911 || code == http.StatusTooManyRequests) {
			return d.startCooldown(retryAfter)
		}
	}
	return nil
}

func (d *Pan115) checkRequest(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	d.cooldownMu.Lock()
	until := d.cooldownUntil
	d.cooldownMu.Unlock()
	if time.Now().Before(until) {
		return fmt.Errorf("%w; requests paused until %s", errRequestLimited, until.UTC().Format(time.RFC3339))
	}
	return nil
}

func (d *Pan115) WaitLimit(ctx context.Context) error {
	if err := d.checkRequest(ctx); err != nil {
		return err
	}
	d.cooldownMu.Lock()
	if d.cooldownChanged == nil {
		d.cooldownChanged = make(chan struct{})
	}
	changed := d.cooldownChanged
	d.cooldownMu.Unlock()
	if err := d.checkRequest(ctx); err != nil {
		return err
	}
	if d.limiter != nil {
		now := time.Now()
		reservation := d.limiter.ReserveN(now, 1)
		if !reservation.OK() {
			return errors.New("115 request exceeds limiter capacity")
		}
		delay := reservation.DelayFrom(now)
		if deadline, ok := ctx.Deadline(); ok && now.Add(delay).After(deadline) {
			reservation.CancelAt(now)
			return context.DeadlineExceeded
		}
		timer := time.NewTimer(delay)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			reservation.Cancel()
			return ctx.Err()
		case <-changed:
			reservation.Cancel()
			return d.checkRequest(ctx)
		case <-timer.C:
		}
	}
	// Another request may have triggered cooldown while this one was queued.
	return d.checkRequest(ctx)
}

func (d *Pan115) startCooldown(retryAfter string) error {
	now := time.Now()
	until := now.Add(defaultCooldown)
	if seconds, err := strconv.ParseInt(retryAfter, 10, 64); err == nil {
		// Bound conversion before multiplying to avoid time.Duration overflow.
		if seconds > 0 && seconds <= int64((1<<63-1)/time.Second) {
			if deadline := now.Add(time.Duration(seconds) * time.Second); deadline.After(until) {
				until = deadline
			}
		}
	} else if deadline, err := http.ParseTime(retryAfter); err == nil && deadline.After(until) {
		until = deadline
	}
	d.cooldownMu.Lock()
	if until.After(d.cooldownUntil) {
		d.cooldownUntil = until
	}
	if d.cooldownChanged != nil {
		close(d.cooldownChanged)
	}
	d.cooldownChanged = make(chan struct{})
	d.cooldownMu.Unlock()
	return d.checkRequest(context.Background())
}

type limitedTransport struct {
	driver *Pan115
	base   http.RoundTripper
}

func (t *limitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.driver.WaitLimit(req.Context()); err != nil {
		return nil, err
	}
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		return resp, err
	}
	// Detect HTTP-level blocks before Resty attempts to parse an HTML response.
	if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode == http.StatusMethodNotAllowed {
		err = t.driver.startCooldown(resp.Header.Get("Retry-After"))
		if closeErr := resp.Body.Close(); closeErr != nil {
			return nil, errors.Wrap(err, closeErr.Error())
		}
		return nil, err
	}
	return resp, nil
}

func (t *limitedTransport) CloseIdleConnections() {
	if closer, ok := t.base.(interface{ CloseIdleConnections() }); ok {
		closer.CloseIdleConnections()
	}
}
