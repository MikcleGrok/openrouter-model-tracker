package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
)

// validSignalMap returns a fresh map[string]any matching plan 4.6's worked
// example / httpapi/dto.go's signalResponseDTO byte-for-byte — the baseline
// every malformed-input test below starts from and mutates one thing at a
// time. Returned fresh every call so one test's mutation never leaks into
// another's.
func validSignalMap() map[string]any {
	return map[string]any{
		"model_key":    "acme/model-1",
		"signal_scope": "community",
		"position":     map[string]any{"community_position": 3},
		"overall": map[string]any{
			"value":        4.2,
			"status":       "established",
			"confidence":   "established",
			"sample_count": 20,
		},
		"skills": map[string]any{
			"reasoning": map[string]any{
				"value":        4.4,
				"status":       "provisional",
				"confidence":   "provisional",
				"sample_count": 12,
			},
		},
		"status": "usable",
		"freshness": map[string]any{
			"as_of":       "2026-01-01T00:00:00Z",
			"computed_at": "2026-01-01T12:00:00Z",
			"ttl_seconds": 86400,
			"stale":       false,
		},
		"schema_version": "feedback-signal.v1",
		"policy_version": "feedback-signal-policy.v1",
	}
}

// serveJSON starts an httptest.Server answering every request with status
// and body JSON-encoded.
func serveJSON(t *testing.T, status int, body any) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if body != nil {
			_ = json.NewEncoder(w).Encode(body)
		}
	}))
}

// serveRaw starts an httptest.Server answering every request with status
// and raw, exactly as given (used for a body that is not even valid JSON).
func serveRaw(t *testing.T, status int, raw string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(status)
		_, _ = io.WriteString(w, raw)
	}))
}

func newTestProvider(t *testing.T, baseURL string) *HTTPReferenceSignalProvider {
	t.Helper()
	p, err := NewHTTPReferenceSignalProvider(HTTPReferenceSignalProviderConfig{BaseURL: baseURL, Token: "test-token"})
	if err != nil {
		t.Fatalf("NewHTTPReferenceSignalProvider: %v", err)
	}
	return p
}

func TestNewHTTPReferenceSignalProvider_Validation(t *testing.T) {
	cases := []struct {
		name string
		cfg  HTTPReferenceSignalProviderConfig
	}{
		{"empty base url", HTTPReferenceSignalProviderConfig{Token: "t"}},
		{"invalid base url", HTTPReferenceSignalProviderConfig{BaseURL: "://bad", Token: "t"}},
		{"non-http scheme", HTTPReferenceSignalProviderConfig{BaseURL: "ftp://example.com", Token: "t"}},
		{"no host", HTTPReferenceSignalProviderConfig{BaseURL: "http://", Token: "t"}},
		{"empty token", HTTPReferenceSignalProviderConfig{BaseURL: "http://example.com", Token: ""}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := NewHTTPReferenceSignalProvider(tc.cfg); err == nil {
				t.Errorf("expected an error for %s", tc.name)
			}
		})
	}
}

// TestGetSignal_Unauthorized covers both HTTP statuses plan 4.5/6.3 map to
// a rejected consumer credential.
func TestGetSignal_Unauthorized(t *testing.T) {
	for _, status := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv := serveJSON(t, status, map[string]any{"error": "nope"})
			defer srv.Close()
			_, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1")
			if !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("err = %v, want ErrUnauthorized", err)
			}
		})
	}
}

// TestGetSignal_UnavailableStatuses covers 5xx plus every other
// HTTP status this adapter does not otherwise recognize.
func TestGetSignal_UnavailableStatuses(t *testing.T) {
	for _, status := range []int{500, 502, 503, 400, 404, 429} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			srv := serveJSON(t, status, map[string]any{"error": "nope"})
			defer srv.Close()
			_, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1")
			if !errors.Is(err, ErrUnavailable) {
				t.Fatalf("err = %v, want ErrUnavailable", err)
			}
		})
	}
}

// TestGetSignal_NetworkError proves a connection failure (nothing listening
// at all) maps to ErrUnavailable.
func TestGetSignal_NetworkError(t *testing.T) {
	srv := serveJSON(t, http.StatusOK, validSignalMap())
	addr := srv.URL
	srv.Close() // nothing is listening at addr anymore
	_, err := newTestProvider(t, addr).GetSignal(context.Background(), "acme/model-1")
	if !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// TestGetSignal_Timeout proves a request exceeding RequestTimeout maps to
// ErrUnavailable, driven via a real, slow httptest.Server rather than a
// mock.
func TestGetSignal_Timeout(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	p, err := NewHTTPReferenceSignalProvider(HTTPReferenceSignalProviderConfig{
		BaseURL:        srv.URL,
		Token:          "test-token",
		RequestTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewHTTPReferenceSignalProvider: %v", err)
	}
	if _, err := p.GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

// TestGetSignal_CallerContextDeadline proves a short-deadline ctx passed by
// the caller (rather than this provider's own RequestTimeout) also maps to
// ErrUnavailable.
func TestGetSignal_CallerContextDeadline(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
	}))
	defer srv.Close()
	p := newTestProvider(t, srv.URL)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := p.GetSignal(ctx, "acme/model-1"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("err = %v, want ErrUnavailable", err)
	}
}

func TestGetSignal_MalformedJSON(t *testing.T) {
	srv := serveRaw(t, http.StatusOK, "{not json")
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

// TestGetSignal_MissingRequiredField removes each required top-level key
// one at a time and confirms every one of them is actually enforced.
func TestGetSignal_MissingRequiredField(t *testing.T) {
	for _, key := range requiredSignalKeys {
		t.Run(key, func(t *testing.T) {
			m := validSignalMap()
			delete(m, key)
			srv := serveJSON(t, http.StatusOK, m)
			defer srv.Close()
			if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
				t.Fatalf("missing %q: err = %v, want ErrIncompatibleSchema", key, err)
			}
		})
	}
}

func TestGetSignal_OverallMissingDimensionKey(t *testing.T) {
	for _, key := range requiredDimensionKeys {
		t.Run(key, func(t *testing.T) {
			m := validSignalMap()
			overall := m["overall"].(map[string]any)
			delete(overall, key)
			srv := serveJSON(t, http.StatusOK, m)
			defer srv.Close()
			if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
				t.Fatalf("overall missing %q: err = %v, want ErrIncompatibleSchema", key, err)
			}
		})
	}
}

// TestGetSignal_FreshnessMissingKey is the regression test for the review's
// "consumer adapter doesn't validate freshness sub-keys" finding: removing
// any of freshness's four expected keys — including "stale", whose absence
// would otherwise silently decode as the Go zero value false, defeating the
// Freshness.Stale defense-in-depth check — must be rejected exactly like a
// missing overall/skills dimension key.
func TestGetSignal_FreshnessMissingKey(t *testing.T) {
	for _, key := range requiredFreshnessKeys {
		t.Run(key, func(t *testing.T) {
			m := validSignalMap()
			freshness := m["freshness"].(map[string]any)
			delete(freshness, key)
			srv := serveJSON(t, http.StatusOK, m)
			defer srv.Close()
			if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
				t.Fatalf("freshness missing %q: err = %v, want ErrIncompatibleSchema", key, err)
			}
		})
	}
}

func TestGetSignal_UnsupportedSchemaVersion(t *testing.T) {
	m := validSignalMap()
	m["schema_version"] = "feedback-signal.v2"
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

// TestGetSignal_UnsupportedPolicyVersion is the regression test for the
// review finding that policy_version was decoded but never validated:
// policy.go's whole safety argument rests on delegating threshold/TTL/
// confidence/routing-safety semantics to server-side policy, which is only
// sound if this adapter pins which policy version those band meanings came
// from. A server claiming a different policy_version could redefine what
// "established" means without changing the wire shape at all.
func TestGetSignal_UnsupportedPolicyVersion(t *testing.T) {
	m := validSignalMap()
	m["policy_version"] = "feedback-signal-policy.v2"
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

func TestGetSignal_UnsupportedSignalScope(t *testing.T) {
	m := validSignalMap()
	m["signal_scope"] = "personal" // there is no personal consumer scope (doc.go)
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

func TestGetSignal_UnrecognizedTopLevelStatus(t *testing.T) {
	m := validSignalMap()
	m["status"] = "something_new"
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

// TestGetSignal_PolicyRejectedStatus and TestGetSignal_IncompatibleSchemaStatus
// cover the two documented-but-not-yet-emitted top-level statuses (plan
// 4.6's enum names them; today's real server never sets them — see
// http_provider.go's decodeSignal comment).
func TestGetSignal_PolicyRejectedStatus(t *testing.T) {
	m := validSignalMap()
	m["status"] = "policy_rejected"
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrPolicyRejected) {
		t.Fatalf("err = %v, want ErrPolicyRejected", err)
	}
}

func TestGetSignal_IncompatibleSchemaStatus(t *testing.T) {
	m := validSignalMap()
	m["status"] = "incompatible_schema"
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

func TestGetSignal_UnrecognizedDimensionStatus(t *testing.T) {
	m := validSignalMap()
	m["overall"].(map[string]any)["status"] = "weird"
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

func TestGetSignal_UnrecognizedSkillDimensionStatus(t *testing.T) {
	m := validSignalMap()
	m["skills"].(map[string]any)["reasoning"].(map[string]any)["status"] = "weird"
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

func TestGetSignal_UnrecognizedConfidence(t *testing.T) {
	m := validSignalMap()
	m["overall"].(map[string]any)["confidence"] = "super-sure"
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); !errors.Is(err, ErrIncompatibleSchema) {
		t.Fatalf("err = %v, want ErrIncompatibleSchema", err)
	}
}

// TestGetSignal_NullSemantics proves an "insufficient" dimension within an
// otherwise-usable signal decodes with a nil Value and a non-nil
// Confidence, matching plan 4.6's "value становится null при недоступности
// dimension" without also nulling confidence for an ordinary insufficient
// count.
func TestGetSignal_NullSemantics(t *testing.T) {
	m := validSignalMap()
	m["overall"] = map[string]any{"value": nil, "status": "insufficient", "confidence": "insufficient", "sample_count": 2}
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	signal, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1")
	if err != nil {
		t.Fatalf("GetSignal: %v", err)
	}
	if signal.Overall.Value != nil {
		t.Errorf("overall.value = %v, want nil", *signal.Overall.Value)
	}
	if signal.Overall.Confidence == nil || *signal.Overall.Confidence != ConfidenceInsufficient {
		t.Errorf("overall.confidence = %v, want insufficient", signal.Overall.Confidence)
	}
	if signal.Overall.SampleCount != 2 {
		t.Errorf("overall.sample_count = %d, want 2", signal.Overall.SampleCount)
	}
}

// TestGetSignal_NullConfidenceForStaleDimension proves a stale dimension's
// confidence decodes as nil, not a fabricated string (plan 4.6: "confidence
// ... либо null для error/stale status").
func TestGetSignal_NullConfidenceForStaleDimension(t *testing.T) {
	m := validSignalMap()
	m["status"] = "stale"
	staleDim := map[string]any{"value": nil, "status": "stale", "confidence": nil, "sample_count": 20}
	m["overall"] = staleDim
	m["skills"] = map[string]any{"reasoning": staleDim}
	m["freshness"].(map[string]any)["stale"] = true
	srv := serveJSON(t, http.StatusOK, m)
	defer srv.Close()
	signal, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1")
	if err != nil {
		t.Fatalf("GetSignal: %v", err)
	}
	if signal.Overall.Confidence != nil {
		t.Errorf("overall.confidence = %v, want nil", *signal.Overall.Confidence)
	}
	if signal.Overall.Value != nil {
		t.Errorf("overall.value = %v, want nil", *signal.Overall.Value)
	}
	if !signal.Freshness.Stale {
		t.Error("freshness.stale = false, want true")
	}
}

func TestGetSignal_SendsBearerAuthHeader(t *testing.T) {
	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(validSignalMap())
	}))
	defer srv.Close()
	p, err := NewHTTPReferenceSignalProvider(HTTPReferenceSignalProviderConfig{BaseURL: srv.URL, Token: "my-secret-token"})
	if err != nil {
		t.Fatalf("NewHTTPReferenceSignalProvider: %v", err)
	}
	if _, err := p.GetSignal(context.Background(), "acme/model-1"); err != nil {
		t.Fatalf("GetSignal: %v", err)
	}
	if gotAuth != "Bearer my-secret-token" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Bearer my-secret-token")
	}
}

// TestGetSignal_ModelKeyPathUnescaped proves a model key's internal "/"
// reaches the server unescaped, matching httpapi/routes.go's "{rest...}"
// wildcard expectation (the same rationale
// internal/feedback/client/transport.go's own modelFeedbackPath documents).
func TestGetSignal_ModelKeyPathUnescaped(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(validSignalMap())
	}))
	defer srv.Close()
	if _, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1"); err != nil {
		t.Fatalf("GetSignal: %v", err)
	}
	if want := "/v1/models/acme/model-1/feedback/signal"; gotPath != want {
		t.Errorf("request path = %q, want %q", gotPath, want)
	}
}

func TestGetSignal_EmptyModelKey(t *testing.T) {
	p := newTestProvider(t, "http://127.0.0.1:1")
	_, err := p.GetSignal(context.Background(), "")
	if err == nil {
		t.Fatal("expected an error for an empty model key")
	}
	if errors.Is(err, ErrUnavailable) || errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrIncompatibleSchema) || errors.Is(err, ErrPolicyRejected) {
		t.Errorf("an empty model key is a caller-contract violation, not one of the four typed adapter errors: got %v", err)
	}
}

// TestGetSignal_Success_MapsAllFields is the adapter's own field-by-field
// mapping proof against a hand-built body matching plan 4.6's contract
// exactly (the real-server equivalent lives in contract_test.go).
func TestGetSignal_Success_MapsAllFields(t *testing.T) {
	srv := serveJSON(t, http.StatusOK, validSignalMap())
	defer srv.Close()
	signal, err := newTestProvider(t, srv.URL).GetSignal(context.Background(), "acme/model-1")
	if err != nil {
		t.Fatalf("GetSignal: %v", err)
	}
	if signal.ModelKey != "acme/model-1" {
		t.Errorf("model_key = %q, want acme/model-1", signal.ModelKey)
	}
	if signal.SignalScope != "community" {
		t.Errorf("signal_scope = %q, want community", signal.SignalScope)
	}
	if signal.CommunityPosition == nil || *signal.CommunityPosition != 3 {
		t.Errorf("community_position = %v, want 3", signal.CommunityPosition)
	}
	if signal.Overall.Status != DimensionEstablished || signal.Overall.SampleCount != 20 {
		t.Errorf("overall = %+v, want established/20", signal.Overall)
	}
	if signal.Overall.Value == nil || *signal.Overall.Value != 4.2 {
		t.Errorf("overall.value = %v, want 4.2", signal.Overall.Value)
	}
	if signal.Overall.Confidence == nil || *signal.Overall.Confidence != ConfidenceEstablished {
		t.Errorf("overall.confidence = %v, want established", signal.Overall.Confidence)
	}
	reasoning, ok := signal.Skills["reasoning"]
	if !ok || reasoning.Status != DimensionProvisional || reasoning.SampleCount != 12 {
		t.Errorf("skills[reasoning] = %+v (present=%v), want provisional/12", reasoning, ok)
	}
	if signal.Status != SignalUsable {
		t.Errorf("status = %q, want usable", signal.Status)
	}
	if signal.Freshness.AsOf == nil {
		t.Error("freshness.as_of is nil, want a value")
	}
	if signal.Freshness.TTLSeconds != 86400 || signal.Freshness.Stale {
		t.Errorf("freshness = %+v, want ttl_seconds=86400 stale=false", signal.Freshness)
	}
	if signal.SchemaVersion != "feedback-signal.v1" || signal.PolicyVersion != "feedback-signal-policy.v1" {
		t.Errorf("versions = %q/%q", signal.SchemaVersion, signal.PolicyVersion)
	}
}

// TestGetSignal_AdapterErrorUnwraps proves an *AdapterError still satisfies
// errors.Is against the sentinel it wraps, so callers can keep matching on
// the package-level Err* variables.
func TestGetSignal_AdapterErrorUnwraps(t *testing.T) {
	err := &AdapterError{Err: ErrUnavailable, Detail: "http status 503"}
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("errors.Is(err, ErrUnavailable) = false, want true")
	}
	if err.Error() == "" {
		t.Error("Error() returned empty string")
	}
}
