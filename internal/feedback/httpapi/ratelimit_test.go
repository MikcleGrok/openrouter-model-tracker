package httpapi

import (
	"bytes"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestRateLimit_WriteEndpointReturns429AfterBurst configures a tiny burst
// so the limit is reached deterministically within a handful of requests,
// rather than waiting on real time.
func TestRateLimit_WriteEndpointReturns429AfterBurst(t *testing.T) {
	env := newTestEnv(t, func(cfg *Config) {
		cfg.RateLimitBurst = 2
		cfg.RateLimitInterval = time.Hour // effectively no refill during the test
	})

	var last *http.Response
	for i := 0; i < 3; i++ {
		body := fmt.Sprintf(`{"overall":%d,"skills":[],"review":""}`, (i%5)+1)
		last = env.do(t, http.MethodPut, "/v1/models/acme/model-1/feedback", userHeaders(identityA), bytes.NewBufferString(body))
		if i < 2 {
			if last.StatusCode != http.StatusOK {
				t.Fatalf("request %d (within burst): status = %d, want 200", i, last.StatusCode)
			}
			last.Body.Close()
		}
	}
	defer last.Body.Close()
	if last.StatusCode != http.StatusTooManyRequests {
		t.Errorf("request past burst: status = %d, want 429", last.StatusCode)
	}
}

// TestRateLimit_ReadEndpointsAreNotLimited confirms plan 10.3's "per-token
// ... на запись" scope: GET routes are never subject to the write rate
// limiter, however small its burst.
func TestRateLimit_ReadEndpointsAreNotLimited(t *testing.T) {
	env := newTestEnv(t, func(cfg *Config) {
		cfg.RateLimitBurst = 1
		cfg.RateLimitInterval = time.Hour
	})
	for i := 0; i < 5; i++ {
		resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityA), nil)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET request %d: status = %d, want 200 (reads are never rate-limited)", i, resp.StatusCode)
		}
	}
}

// TestConcurrency_RejectsBeyondMaxInFlight proves the concurrency limiter
// (plan 10.1) with a server capacity of exactly 1: a request arriving while
// the single slot is already occupied gets 503 immediately rather than
// being queued. It exercises the limiter through the server's own inFlight
// channel directly (the same channel concurrencyMiddleware selects on) to
// get a deterministic "capacity already full" state without depending on
// real request timing/goroutine races.
func TestConcurrency_RejectsBeyondMaxInFlight(t *testing.T) {
	env := newTestEnv(t, func(cfg *Config) { cfg.MaxInFlight = 1 })

	env.server.inFlight <- struct{}{} // occupy the only slot
	defer func() { <-env.server.inFlight }()

	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityA), nil)
	resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("request while at MaxInFlight capacity: status = %d, want 503", resp.StatusCode)
	}
}
