package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// testEnv wires a Client at an httptest.Server whose handler is supplied per
// test. It also writes a valid token_file/identity_file for the client to
// read, and exposes the identity so tests can assert it round-trips into
// the X-Identity-Id header.
type testEnv struct {
	client   *Client
	identity string
	token    string
	server   *httptest.Server
}

func newTestEnv(t *testing.T, handler http.HandlerFunc) *testEnv {
	t.Helper()
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")

	const token = "test-shared-secret"
	if err := os.WriteFile(tokenFile, []byte(token+"\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	identity, _, err := EnsureIdentityFile(identityFile)
	if err != nil {
		t.Fatalf("EnsureIdentityFile: %v", err)
	}

	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	c, err := New(Config{
		Endpoint:       server.URL,
		TokenFile:      tokenFile,
		IdentityFile:   identityFile,
		RequestTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return &testEnv{client: c, identity: identity, token: token, server: server}
}

func writeJSONFixture(t *testing.T, w http.ResponseWriter, status int, body string) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if _, err := w.Write([]byte(body)); err != nil {
		t.Fatalf("write fixture response: %v", err)
	}
}

// requireAuthHeaders fails the test unless r carries exactly the auth
// headers this client must always send (plan 6.1/httpapi/auth.go).
func requireAuthHeaders(t *testing.T, r *http.Request, wantToken, wantIdentity string) {
	t.Helper()
	if got := r.Header.Get("Authorization"); got != "Bearer "+wantToken {
		t.Errorf("Authorization header = %q, want %q", got, "Bearer "+wantToken)
	}
	if got := r.Header.Get("X-Identity-Id"); got != wantIdentity {
		t.Errorf("X-Identity-Id header = %q, want %q", got, wantIdentity)
	}
}

// --- PutFeedback ---

func TestPutFeedback_SendsExactWireShapeAndParsesSummary(t *testing.T) {
	var capturedBody map[string]any
	var capturedPath, capturedMethod, capturedContentType string
	var env *testEnv
	env = newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		capturedMethod = r.Method
		capturedContentType = r.Header.Get("Content-Type")
		requireAuthHeaders(t, r, env.token, env.identity)
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode request body: %v", err)
		}
		writeJSONFixture(t, w, http.StatusOK, `{
			"model_key": "acme/model-1",
			"mine": {"overall":4,"skills":[{"key":"reasoning","rating":5}],"review":"nice","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-01T00:00:00Z"},
			"community": {"count":1,"average":4,"distribution":[0,0,0,1,0],"skills":[{"key":"reasoning","count":1,"average":5}],"computed_at":"2026-01-01T00:00:00Z"},
			"base_position": {"status":"unranked"},
			"personal_position": {"value":1,"status":"ranked"},
			"community_position": {"status":"ineligible"}
		}`)
	})

	got, err := env.client.PutFeedback(context.Background(), "acme/model-1", FeedbackRequest{
		Overall: 4,
		Skills:  []SkillRating{{Key: "reasoning", Rating: 5}},
		Review:  "nice",
	})
	if err != nil {
		t.Fatalf("PutFeedback: %v", err)
	}

	if capturedMethod != http.MethodPut {
		t.Errorf("method = %q, want PUT", capturedMethod)
	}
	if capturedPath != "/v1/models/acme/model-1/feedback" {
		t.Errorf("path = %q, want /v1/models/acme/model-1/feedback", capturedPath)
	}
	if capturedContentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", capturedContentType)
	}
	if capturedBody["overall"] != float64(4) || capturedBody["review"] != "nice" {
		t.Errorf("request body = %+v, want overall=4 review=nice", capturedBody)
	}
	skills, _ := capturedBody["skills"].([]any)
	if len(skills) != 1 {
		t.Fatalf("request body skills = %+v, want 1 entry", capturedBody["skills"])
	}
	if _, hasModelKey := capturedBody["model_key"]; hasModelKey {
		t.Error("request body must never include model_key (it belongs in the URL only)")
	}

	if got.ModelKey != "acme/model-1" {
		t.Errorf("ModelKey = %q", got.ModelKey)
	}
	if got.Mine == nil || got.Mine.Overall != 4 || got.Mine.Review != "nice" {
		t.Errorf("Mine = %+v", got.Mine)
	}
	if got.Community == nil || got.Community.Count != 1 || got.Community.Average == nil || *got.Community.Average != 4 {
		t.Errorf("Community = %+v", got.Community)
	}
	if got.BasePosition.Status != PositionUnranked {
		t.Errorf("BasePosition.Status = %q, want unranked", got.BasePosition.Status)
	}
	if got.PersonalPosition.Status != PositionRanked || got.PersonalPosition.Value != 1 {
		t.Errorf("PersonalPosition = %+v, want ranked/1", got.PersonalPosition)
	}
	if got.CommunityPosition.Status != PositionIneligible {
		t.Errorf("CommunityPosition.Status = %q, want ineligible", got.CommunityPosition.Status)
	}
	if got.Others != nil {
		t.Errorf("Others = %+v, want nil (server never populates it on PUT)", got.Others)
	}
}

func TestPutFeedback_NilSkillsMarshalAsEmptyArrayNotNull(t *testing.T) {
	var rawBody string
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		data, err := io.ReadAll(r.Body)
		if err != nil {
			t.Fatalf("read request body: %v", err)
		}
		rawBody = string(data)
		writeJSONFixture(t, w, http.StatusOK, `{"model_key":"m","mine":null,"community":{"count":0,"average":null,"distribution":[0,0,0,0,0],"skills":[],"computed_at":"2026-01-01T00:00:00Z"},"base_position":{"status":"unranked"},"personal_position":{"status":"unranked"},"community_position":{"status":"ineligible"}}`)
	})
	_, err := env.client.PutFeedback(context.Background(), "m", FeedbackRequest{Overall: 3})
	if err != nil {
		t.Fatalf("PutFeedback: %v", err)
	}
	if want := `"skills":[]`; !strings.Contains(rawBody, want) {
		t.Errorf("request body = %s, want it to contain %q (never null)", rawBody, want)
	}
}

// --- GetOwnFeedback ---

func TestGetOwnFeedback_NoRatingYetReturnsNilWithoutError(t *testing.T) {
	var capturedPath, capturedMethod string
	var env *testEnv
	env = newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath, capturedMethod = r.URL.Path, r.Method
		requireAuthHeaders(t, r, env.token, env.identity)
		writeJSONFixture(t, w, http.StatusOK, `{"model_key":"acme/model-1","own_feedback":null}`)
	})

	got, err := env.client.GetOwnFeedback(context.Background(), "acme/model-1")
	if err != nil {
		t.Fatalf("GetOwnFeedback: %v", err)
	}
	if capturedMethod != http.MethodGet {
		t.Errorf("method = %q, want GET", capturedMethod)
	}
	if capturedPath != "/v1/models/acme/model-1/feedback/me" {
		t.Errorf("path = %q, want /v1/models/acme/model-1/feedback/me", capturedPath)
	}
	if got.ModelKey != "acme/model-1" || got.OwnFeedback != nil {
		t.Errorf("got = %+v, want model_key=acme/model-1 own_feedback=nil", got)
	}
}

func TestGetOwnFeedback_WithRatingParsesFully(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONFixture(t, w, http.StatusOK, `{"model_key":"acme/model-1","own_feedback":{"overall":2,"skills":[],"review":"","created_at":"2026-01-01T00:00:00Z","updated_at":"2026-01-02T00:00:00Z"}}`)
	})
	got, err := env.client.GetOwnFeedback(context.Background(), "acme/model-1")
	if err != nil {
		t.Fatalf("GetOwnFeedback: %v", err)
	}
	if got.OwnFeedback == nil || got.OwnFeedback.Overall != 2 {
		t.Errorf("OwnFeedback = %+v", got.OwnFeedback)
	}
	wantUpdated := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	if !got.OwnFeedback.UpdatedAt.Equal(wantUpdated) {
		t.Errorf("UpdatedAt = %v, want %v", got.OwnFeedback.UpdatedAt, wantUpdated)
	}
}

// --- GetSummary ---

func TestGetSummary_OthersOmittedByDefaultIncludedWhenRequested(t *testing.T) {
	var capturedQuery string
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		if r.URL.Query().Get("others") == "true" {
			writeJSONFixture(t, w, http.StatusOK, `{"model_key":"m","mine":null,"community":{"count":2,"average":3,"distribution":[0,0,2,0,0],"skills":[],"computed_at":"2026-01-01T00:00:00Z"},"others":{"count":1,"average":1,"distribution":[1,0,0,0,0],"skills":[],"computed_at":"2026-01-01T00:00:00Z"},"base_position":{"status":"unranked"},"personal_position":{"status":"unranked"},"community_position":{"status":"ineligible"}}`)
			return
		}
		writeJSONFixture(t, w, http.StatusOK, `{"model_key":"m","mine":null,"community":{"count":2,"average":3,"distribution":[0,0,2,0,0],"skills":[],"computed_at":"2026-01-01T00:00:00Z"},"base_position":{"status":"unranked"},"personal_position":{"status":"unranked"},"community_position":{"status":"ineligible"}}`)
	})

	without, err := env.client.GetSummary(context.Background(), "m", false)
	if err != nil {
		t.Fatalf("GetSummary(false): %v", err)
	}
	if capturedQuery != "" {
		t.Errorf("query = %q, want empty when includeOthers=false", capturedQuery)
	}
	if without.Others != nil {
		t.Errorf("Others = %+v, want nil when not requested", without.Others)
	}

	with, err := env.client.GetSummary(context.Background(), "m", true)
	if err != nil {
		t.Fatalf("GetSummary(true): %v", err)
	}
	if capturedQuery != "others=true" {
		t.Errorf("query = %q, want others=true", capturedQuery)
	}
	if with.Others == nil || with.Others.Count != 1 {
		t.Errorf("Others = %+v, want count=1", with.Others)
	}
}

func TestGetSummary_ModelKeyWithSlashPreservedInPath(t *testing.T) {
	var capturedPath string
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		writeJSONFixture(t, w, http.StatusOK, `{"model_key":"anthropic/claude-3","mine":null,"community":{"count":0,"average":null,"distribution":[0,0,0,0,0],"skills":[],"computed_at":"2026-01-01T00:00:00Z"},"base_position":{"status":"unranked"},"personal_position":{"status":"unranked"},"community_position":{"status":"ineligible"}}`)
	})
	if _, err := env.client.GetSummary(context.Background(), "anthropic/claude-3", false); err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if capturedPath != "/v1/models/anthropic/claude-3/feedback/summary" {
		t.Errorf("path = %q, want /v1/models/anthropic/claude-3/feedback/summary (raw, unescaped slash)", capturedPath)
	}
}

// --- Non-2xx status mapping ---

func TestNon2xxStatusMapping(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		check  func(t *testing.T, err error)
	}{
		{"400 validation", http.StatusBadRequest, `{"error":"validation failed","fields":[{"field":"overall","value":"9","message":"must be an integer between 1 and 5"}]}`, func(t *testing.T, err error) {
			if !IsValidationError(err) {
				t.Fatalf("IsValidationError(%v) = false", err)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v, want *APIError", err)
			}
			if len(apiErr.Fields) != 1 || apiErr.Fields[0].Field != "overall" {
				t.Errorf("Fields = %+v", apiErr.Fields)
			}
		}},
		{"401 unauthorized", http.StatusUnauthorized, `{"error":"missing or invalid bearer token"}`, func(t *testing.T, err error) {
			if !IsUnauthorized(err) {
				t.Fatalf("IsUnauthorized(%v) = false", err)
			}
		}},
		{"403 forbidden", http.StatusForbidden, `{"error":"token is not authorized for this endpoint"}`, func(t *testing.T, err error) {
			if !IsForbidden(err) {
				t.Fatalf("IsForbidden(%v) = false", err)
			}
		}},
		{"413 too large", http.StatusRequestEntityTooLarge, `{"error":"request body too large"}`, func(t *testing.T, err error) {
			if !IsTooLarge(err) {
				t.Fatalf("IsTooLarge(%v) = false", err)
			}
		}},
		{"429 rate limited", http.StatusTooManyRequests, `{"error":"rate limit exceeded"}`, func(t *testing.T, err error) {
			if !IsRateLimited(err) {
				t.Fatalf("IsRateLimited(%v) = false", err)
			}
		}},
		{"503 unavailable", http.StatusServiceUnavailable, `{"error":"service temporarily unavailable"}`, func(t *testing.T, err error) {
			if !IsServiceUnavailable(err) {
				t.Fatalf("IsServiceUnavailable(%v) = false", err)
			}
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
				writeJSONFixture(t, w, tc.status, tc.body)
			})
			_, err := env.client.PutFeedback(context.Background(), "m", FeedbackRequest{Overall: 3})
			if err == nil {
				t.Fatalf("PutFeedback with status %d = nil error, want an error", tc.status)
			}
			var apiErr *APIError
			if !errors.As(err, &apiErr) {
				t.Fatalf("err = %v (%T), want *APIError", err, err)
			}
			if apiErr.StatusCode != tc.status {
				t.Errorf("StatusCode = %d, want %d", apiErr.StatusCode, tc.status)
			}
			tc.check(t, err)
		})
	}
}

// --- DELETE 204/202/503 distinction ---

func TestDeleteMe_204MeansDone(t *testing.T) {
	var capturedPath, capturedMethod string
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		capturedPath, capturedMethod = r.URL.Path, r.Method
		w.WriteHeader(http.StatusNoContent)
	})
	result, err := env.client.DeleteMe(context.Background())
	if err != nil {
		t.Fatalf("DeleteMe: %v", err)
	}
	if result.Pending {
		t.Error("Pending = true, want false for 204")
	}
	if capturedMethod != http.MethodDelete || capturedPath != "/v1/me/feedback" {
		t.Errorf("method/path = %s %s, want DELETE /v1/me/feedback", capturedMethod, capturedPath)
	}
}

func TestDeleteMe_202MeansCommittedButPending(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONFixture(t, w, http.StatusAccepted, `{"status":"cleanup_pending"}`)
	})
	result, err := env.client.DeleteMe(context.Background())
	if err != nil {
		t.Fatalf("DeleteMe: %v, want nil error for 202 (it is a success, not a failure)", err)
	}
	if !result.Pending {
		t.Error("Pending = false, want true for 202 cleanup_pending")
	}
}

func TestDeleteMe_503MeansFailureNotPending(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSONFixture(t, w, http.StatusServiceUnavailable, `{"error":"internal error"}`)
	})
	result, err := env.client.DeleteMe(context.Background())
	if err == nil {
		t.Fatal("DeleteMe with 503 = nil error, want an error (never treat as pending)")
	}
	if result.Pending {
		t.Error("Pending = true on a 503 failure, want false (zero value)")
	}
	if !IsServiceUnavailable(err) {
		t.Errorf("IsServiceUnavailable(%v) = false", err)
	}
}

// --- Timeout / network error mapping ---

func TestDo_ContextDeadlineMapsToTimeoutError(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeJSONFixture(t, w, http.StatusOK, `{}`)
	})
	client, err := New(Config{
		Endpoint:       env.server.URL,
		TokenFile:      env.client.tokenFile,
		IdentityFile:   env.client.identityFile,
		RequestTimeout: 20 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = client.GetOwnFeedback(context.Background(), "m")
	if err == nil {
		t.Fatal("GetOwnFeedback with a 20ms timeout against a 200ms handler = nil error, want a timeout error")
	}
	if !IsTimeout(err) {
		t.Errorf("IsTimeout(%v) = false, want true", err)
	}
	if !IsNetworkError(err) {
		t.Errorf("IsNetworkError(%v) = false, want true (a timeout counts as a network error too)", err)
	}
}

func TestDo_UnreachableServerMapsToNetworkError(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")
	if err := os.WriteFile(tokenFile, []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureIdentityFile(identityFile); err != nil {
		t.Fatal(err)
	}
	// Port 0 on a resolved loopback address that nothing listens on: use a
	// closed server instead, which reliably refuses the connection.
	server := httptest.NewServer(http.NotFoundHandler())
	unreachable := server.URL
	server.Close() // now guaranteed nothing is listening there

	client, err := New(Config{Endpoint: unreachable, TokenFile: tokenFile, IdentityFile: identityFile, RequestTimeout: time.Second})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = client.GetOwnFeedback(context.Background(), "m")
	if err == nil {
		t.Fatal("GetOwnFeedback against a closed server = nil error, want a network error")
	}
	if !IsNetworkError(err) {
		t.Errorf("IsNetworkError(%v) = false, want true", err)
	}
	if IsTimeout(err) {
		t.Errorf("IsTimeout(%v) = true, want false (connection refused is not a timeout)", err)
	}
}

func TestDo_CallerContextCancellationMapsToTimeoutOrNetworkError(t *testing.T) {
	env := newTestEnv(t, func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(200 * time.Millisecond)
		writeJSONFixture(t, w, http.StatusOK, `{}`)
	})
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	_, err := env.client.GetOwnFeedback(ctx, "m")
	if err == nil {
		t.Fatal("GetOwnFeedback with an already-expiring caller context = nil error, want an error")
	}
	if !IsNetworkError(err) {
		t.Errorf("IsNetworkError(%v) = false, want true", err)
	}
}

// --- Credential errors surface distinctly ---

func TestDo_MissingTokenFileSurfacesCredentialErrorNotNetworkError(t *testing.T) {
	dir := t.TempDir()
	identityFile := filepath.Join(dir, "identity")
	if _, _, err := EnsureIdentityFile(identityFile); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("server should never be contacted when the token file cannot be read")
	}))
	defer server.Close()

	client, err := New(Config{
		Endpoint:     server.URL,
		TokenFile:    filepath.Join(dir, "does-not-exist"),
		IdentityFile: identityFile,
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	_, err = client.GetOwnFeedback(context.Background(), "m")
	if !IsCredentialError(err) {
		t.Fatalf("err = %v (%T), want a *CredentialError", err, err)
	}
	if IsNetworkError(err) {
		t.Error("a missing token file must not be reported as a network error")
	}
}

// --- Constructor validation ---

func TestNew_RejectsInvalidConfig(t *testing.T) {
	dir := t.TempDir()
	// New validates only the shape of Config, never whether these files
	// exist yet (that happens per-request) — so they are never created
	// here.
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")

	cases := []Config{
		{Endpoint: "", TokenFile: tokenFile, IdentityFile: identityFile},
		{Endpoint: "ftp://example.com", TokenFile: tokenFile, IdentityFile: identityFile},
		{Endpoint: "http://", TokenFile: tokenFile, IdentityFile: identityFile},
		{Endpoint: "http://example.com", TokenFile: "", IdentityFile: identityFile},
		{Endpoint: "http://example.com", TokenFile: tokenFile, IdentityFile: ""},
	}
	for i, cfg := range cases {
		if _, err := New(cfg); err == nil {
			t.Errorf("case %d: New(%+v) = nil error, want an error", i, cfg)
		}
	}
}

func TestNew_DefaultsTimeoutWhenUnset(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")
	c, err := New(Config{Endpoint: "http://example.com", TokenFile: tokenFile, IdentityFile: identityFile})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if c.timeout != DefaultRequestTimeout {
		t.Errorf("timeout = %v, want default %v", c.timeout, DefaultRequestTimeout)
	}
}
