package httpapi

import (
	"io"
	"net/http"
	"testing"
	"time"
)

// TestRequestTimeout_ExpiresBeforeHandlerRuns configures an effectively-zero
// request timeout so http.TimeoutHandler's deadline is certain to have
// already elapsed by the time the spawned handler goroutine is even
// scheduled, making the timeout path deterministic without depending on any
// artificially slow handler or database contention.
func TestRequestTimeout_ExpiresBeforeHandlerRuns(t *testing.T) {
	env := newTestEnv(t, func(cfg *Config) { cfg.RequestTimeout = 1 * time.Nanosecond })

	resp := env.do(t, http.MethodGet, "/v1/models/acme/model-1/feedback/me", userHeaders(identityA), nil)
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("request with ~0 timeout: status = %d, want 503", resp.StatusCode)
	}
	body, _ := io.ReadAll(resp.Body)
	if string(body) == "" {
		t.Errorf("timeout response body is empty, want a generic message")
	}
}
