package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestRecoverMiddleware_TurnsPanicIntoOne500 exercises recoverMiddleware
// directly against a handler that deliberately panics — standing in for
// internal/feedback/ranking.go's two documented "should never happen"
// panics, which cannot be triggered through the public HTTP surface without
// an actual internal bug. This proves the middleware itself does what
// Handler's doc comment requires: a panic anywhere in the wrapped handler
// becomes exactly one 500 response, not a crashed process.
func TestRecoverMiddleware_TurnsPanicIntoOne500(t *testing.T) {
	env := newTestEnv(t, nil)

	panicky := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("simulated internal invariant violation")
	})
	handler := env.server.recoverMiddleware(panicky)

	req := httptest.NewRequest(http.MethodGet, "/whatever", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	got := decodeJSON[errorResponseDTO](t, rec.Result())
	if got.Error == "" {
		t.Errorf("error message is empty")
	}
}
