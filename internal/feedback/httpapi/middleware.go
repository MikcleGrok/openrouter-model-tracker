package httpapi

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

// writeJSON writes body as the JSON response with the given status and a
// Content-Type: application/json header (plan 4.5: "все ответы JSON с
// Content-Type: application/json").
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

// writeError writes a generic {"error": message} body. message must always
// be a short, static, safe-to-expose string chosen by the caller — never an
// underlying error's own text on a 5xx path (plan 4.2: "503/500 не должны
// раскрывать SQL, filesystem path или token").
func writeError(w http.ResponseWriter, status int, message string) {
	writeJSON(w, status, errorResponseDTO{Error: message})
}

// writeValidationError writes a structured 400 for one *feedback.ValidationError.
func writeValidationError(w http.ResponseWriter, verr *feedback.ValidationError) {
	writeJSON(w, http.StatusBadRequest, errorResponseDTO{
		Error:  "validation failed",
		Fields: []validationFieldDTO{{Field: verr.Field, Value: verr.Value, Message: verr.Message}},
	})
}

// writeInputError maps an error from feedback.NewFeedbackInput/NormalizeModelKey
// to a structured 400 when it is a *feedback.ValidationError, or a generic
// 400 otherwise (defensive: every current caller only ever produces a
// ValidationError here, but a future validation rule that returns something
// else must not turn into a 500).
func writeInputError(w http.ResponseWriter, err error) {
	var verr *feedback.ValidationError
	if errors.As(err, &verr) {
		writeValidationError(w, verr)
		return
	}
	writeError(w, http.StatusBadRequest, "invalid request")
}

// recoverMiddleware turns a panic anywhere in next into a single 500
// response for that one request, instead of taking down the process. It
// must run in the same goroutine as next (doc.go/server.go's Handler
// wires it directly around routes(), inside http.TimeoutHandler's own
// per-request goroutine) — a recover() in a different goroutine than the
// panic does nothing.
//
// This exists because Service's use cases can panic on what should be
// unreachable invariants (internal/feedback/ranking.go's two documented
// "should never happen" panics) — putting them behind a network server
// means a bug there must degrade to one failed request, not a crashed
// server.
func (s *Server) recoverMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				s.logger.Error("panic recovered while handling request", "panic", fmt.Sprintf("%v", rec), "method", r.Method, "path", r.URL.Path)
				writeError(w, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// concurrencyMiddleware bounds how many requests this server processes at
// once (plan 10.1: "ограничить... количество одновременно обрабатываемых
// запросов"). It never queues: a request arriving when the server is
// already at capacity is rejected immediately with 503 rather than made to
// wait, so a burst of slow requests cannot silently pile up latency for
// everyone behind them.
func (s *Server) concurrencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case s.inFlight <- struct{}{}:
			defer func() { <-s.inFlight }()
			next.ServeHTTP(w, r)
		default:
			writeError(w, http.StatusServiceUnavailable, "server busy")
		}
	})
}
