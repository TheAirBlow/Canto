// Package httpx provides shared HTTP retry, backoff, and lockout primitives for Canto's external metadata sources.
package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"sync"
	"time"
)

// ExternalTransport caps concurrent connections per host, so a burst of import workers doesn't look like abusive traffic to a rate-sensitive third-party service.
var ExternalTransport = &http.Transport{
	MaxConnsPerHost:     4,
	MaxIdleConnsPerHost: 4,
	IdleConnTimeout:     90 * time.Second,
}

// NewExternalClient builds a timeout-bounded client sharing ExternalTransport, for a rate-sensitive third-party API.
func NewExternalClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: ExternalTransport}
}

// imageTransport caps concurrent per-host connections for thumbnail downloads, looser than ExternalTransport since CDNs tolerate more concurrency than scraped metadata APIs.
var imageTransport = &http.Transport{
	MaxConnsPerHost:     8,
	MaxIdleConnsPerHost: 8,
	IdleConnTimeout:     90 * time.Second,
}

// NewImageClient builds a timeout-bounded client sharing imageTransport, for thumbnail downloads.
func NewImageClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, Transport: imageTransport}
}

// statusRetries bounds how many times Do retries a non-final response before returning it as-is.
const statusRetries = 3

// statusRetryDelay is the pause between status-retry attempts.
const statusRetryDelay = 2 * time.Second

// networkBackoffBase is Do's starting delay after a network-level error.
const networkBackoffBase = 1 * time.Second

// networkBackoffMax caps Do's network-error backoff.
const networkBackoffMax = 60 * time.Second

// OKOrNotFound is the common final predicate for Do/DoLocked: stop retrying on a clean 200 or a definitive 404.
func OKOrNotFound(status int) bool { return status == http.StatusOK || status == http.StatusNotFound }

// sleep pauses for d, or returns false early if ctx ends first.
func sleep(ctx context.Context, d time.Duration) bool {
	select {
	case <-time.After(d):
		return true
	case <-ctx.Done():
		return false
	}
}

// Do executes a request freshly built by newReq each attempt, retrying a network error with backoff and a non-final status with a fixed delay, up to statusRetries times each.
func Do(ctx context.Context, client *http.Client, final func(status int) bool, newReq func() (*http.Request, error)) (*http.Response, error) {
	var networkAttempt, statusAttempt int
	for {
		req, err := newReq()
		if err != nil {
			return nil, err
		}
		resp, err := client.Do(req)
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			networkAttempt++
			if networkAttempt >= statusRetries {
				return nil, err
			}
			shift := min(networkAttempt, 6)
			delay := min(networkBackoffBase*(1<<shift), networkBackoffMax)
			slog.Warn("httpx: network error, retrying", "url", req.URL.Redacted(), "attempt", networkAttempt, "delay", delay, "err", err)
			if !sleep(ctx, delay) {
				return nil, ctx.Err()
			}
			continue
		}
		if final(resp.StatusCode) {
			return resp, nil
		}
		statusAttempt++
		if statusAttempt >= statusRetries {
			return resp, nil
		}
		resp.Body.Close()
		if !sleep(ctx, statusRetryDelay) {
			return nil, ctx.Err()
		}
	}
}

// lockoutBase is a Lockout's initial cooldown after a suspected outage.
const lockoutBase = 5 * time.Second

// lockoutMax caps how long consecutive failures can extend a Lockout's cooldown.
const lockoutMax = 5 * time.Minute

// Lockout is a shared cooldown so concurrent callers of one struggling external service back off together instead of continuing to hammer it.
type Lockout struct {
	mu               sync.Mutex
	lockedUntil      time.Time
	consecutiveFails int
}

// Wait blocks until any active cooldown has elapsed, or ctx ends first.
func (l *Lockout) Wait(ctx context.Context) error {
	l.mu.Lock()
	remaining := time.Until(l.lockedUntil)
	l.mu.Unlock()
	if remaining <= 0 {
		return nil
	}
	if !sleep(ctx, remaining) {
		return ctx.Err()
	}
	return nil
}

// Trip engages an exponentially growing cooldown after a suspected outage, logging reason.
func (l *Lockout) Trip(reason string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	shift := min(l.consecutiveFails, 10)
	dur := min(lockoutBase*(1<<shift), lockoutMax)
	l.lockedUntil = time.Now().Add(dur)
	if dur < lockoutMax {
		l.consecutiveFails++
	}
	slog.Warn("httpx: backing off", "reason", reason, "duration", dur, "consecutive", l.consecutiveFails)
}

// Recover resets the consecutive-failure count after a success.
func (l *Lockout) Recover() {
	l.mu.Lock()
	l.consecutiveFails = 0
	l.mu.Unlock()
}

// DoLocked wraps Do with lockout, waiting out any active cooldown first and tripping a new one to retry again if the result still isn't final.
func DoLocked(ctx context.Context, client *http.Client, lockout *Lockout, final func(status int) bool, newReq func() (*http.Request, error)) (*http.Response, error) {
	for {
		if err := lockout.Wait(ctx); err != nil {
			return nil, err
		}
		resp, err := Do(ctx, client, final, newReq)
		if err != nil {
			return nil, err
		}
		if final(resp.StatusCode) {
			lockout.Recover()
			return resp, nil
		}
		resp.Body.Close()
		lockout.Trip(fmt.Sprintf("status %d", resp.StatusCode))
	}
}
