package httpapi

import (
	"net/http"
	"strings"
)

// The four suffixes a request under /v1/models/{...} can end in. A ModelKey
// (contract.md §7) may contain internal "/" characters (typically one, e.g.
// "anthropic/claude-3", but NormalizeModelKey does not actually cap the
// count — it only rejects a leading/trailing "/", "//", and ".."), so Go's
// stdlib ServeMux single-segment wildcards cannot capture it directly:
// {model_key} would only ever match up to the next "/". Instead, every
// route under /v1/models/ is registered once per method using a trailing
// "{rest...}" wildcard that captures everything after "/v1/models/", and
// splitModelKeyPath below recovers the model key by stripping the exact,
// known suffix for whichever logical route matched — which works correctly
// regardless of how many internal slashes the model key contains or how
// the client chose to encode them, since net/http's request path is
// already fully unescaped before pattern matching sees it, and the suffix
// is matched against the tail of the whole captured string, not against
// any particular path segment.
const (
	feedbackSuffix = "/feedback"
	meSuffix       = "/feedback/me"
	summarySuffix  = "/feedback/summary"
	signalSuffix   = "/feedback/signal"
)

// splitModelKeyPath strips suffix from the end of rest and returns what
// remains as the raw (not yet normalized/validated) model key. ok is false
// when rest does not end with suffix, or when nothing is left after
// stripping it (e.g. rest == suffix exactly, meaning the model key segment
// was empty) — both cases mean "this path does not name a real route",
// which callers answer with 404, matching an unmatched net/http pattern.
func splitModelKeyPath(rest, suffix string) (modelKey string, ok bool) {
	if !strings.HasSuffix(rest, suffix) {
		return "", false
	}
	modelKey = strings.TrimSuffix(rest, suffix)
	if modelKey == "" {
		return "", false
	}
	return modelKey, true
}

// routes builds the complete route table (plan 4.2-4.7). Every handler
// function referenced here is defined in handlers_*.go / delete.go.
func (s *Server) routes() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("PUT /v1/models/{rest...}", s.handlePutModels)
	mux.HandleFunc("GET /v1/models/{rest...}", s.handleModelsGet)
	mux.HandleFunc("DELETE /v1/me/feedback", s.authUser(s.handleDeleteMe))
	return mux
}

// handlePutModels is the sole registered PUT handler under /v1/models/: it
// recovers the raw model key from the "{rest...}" wildcard (the only
// logical PUT route is .../feedback) and, once found, hands off to the
// user-auth-gated write handler.
func (s *Server) handlePutModels(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")
	rawModelKey, ok := splitModelKeyPath(rest, feedbackSuffix)
	if !ok {
		http.NotFound(w, r)
		return
	}
	s.authUser(func(w http.ResponseWriter, r *http.Request) {
		s.handlePutFeedback(w, r, rawModelKey)
	})(w, r)
}

// handleModelsGet is the sole registered GET handler under /v1/models/: it
// dispatches on which of the three known GET suffixes rest ends with (own
// feedback, summary, or the trusted-consumer signal) and applies the
// correct auth scope for that specific route — never a shared "GET
// /v1/models/*" auth policy, since exactly one of these three routes
// (.../feedback/signal) belongs to a completely different, non-overlapping
// auth scope than the other two.
func (s *Server) handleModelsGet(w http.ResponseWriter, r *http.Request) {
	rest := r.PathValue("rest")

	if rawModelKey, ok := splitModelKeyPath(rest, signalSuffix); ok {
		s.authConsumer(func(w http.ResponseWriter, r *http.Request) {
			s.handleGetSignal(w, r, rawModelKey)
		})(w, r)
		return
	}
	if rawModelKey, ok := splitModelKeyPath(rest, meSuffix); ok {
		s.authUser(func(w http.ResponseWriter, r *http.Request) {
			s.handleGetOwnFeedback(w, r, rawModelKey)
		})(w, r)
		return
	}
	if rawModelKey, ok := splitModelKeyPath(rest, summarySuffix); ok {
		s.authUser(func(w http.ResponseWriter, r *http.Request) {
			s.handleGetSummary(w, r, rawModelKey)
		})(w, r)
		return
	}
	http.NotFound(w, r)
}
