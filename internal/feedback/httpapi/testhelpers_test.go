package httpapi

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/sqlite"
)

const (
	testUserToken     = "user-secret-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	testConsumerToken = "consumer-secret-bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

	identityA = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	identityB = "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
)

// testEnv bundles a real *sqlite.Store (temp file, fully migrated), the
// httpapi.Server built over it, and an httptest.Server exposing it — every
// test in this package drives real HTTP round trips against a real
// database, per the brief's own instruction (11.2: "поднимать httptest.Server
// с реальным httpapi... и временным SQLite").
type testEnv struct {
	t      *testing.T
	store  *sqlite.Store
	server *Server
	http   *httptest.Server
	now    time.Time // the fixed clock testEnv's Server was built with
}

// newTestEnv opens a fresh temp-file SQLite store, migrates it, builds a
// Server with a fixed clock (mutate env.now and nothing re-reads it
// automatically — see withNow) and both test tokens configured, and starts
// an httptest.Server in front of it. Cleanup closes everything.
func newTestEnv(t *testing.T, configure func(cfg *Config)) *testEnv {
	t.Helper()
	ctx := context.Background()

	dbPath := filepath.Join(t.TempDir(), "feedback.db")
	store, err := sqlite.Open(ctx, dbPath, sqlite.Config{})
	if err != nil {
		t.Fatalf("sqlite.Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })

	if _, err := store.Migrate(ctx, io.Discard); err != nil {
		t.Fatalf("Migrate: %v", err)
	}

	fixedNow := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	env := &testEnv{t: t, store: store, now: fixedNow}

	cfg := Config{
		UserToken:     []byte(testUserToken),
		ConsumerToken: []byte(testConsumerToken),
		BackupDir:     filepath.Join(t.TempDir(), "backups"),
		Now:           func() time.Time { return env.now },
	}
	if configure != nil {
		configure(&cfg)
	}

	server, err := New(store, cfg)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	env.server = server

	env.http = httptest.NewServer(server.Handler())
	t.Cleanup(env.http.Close)

	return env
}

// do issues an HTTP request against env's httptest.Server. body may be nil.
func (env *testEnv) do(t *testing.T, method, path string, headers map[string]string, body io.Reader) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, env.http.URL+path, body)
	if err != nil {
		t.Fatalf("NewRequest: %v", err)
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := env.http.Client().Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

// userHeaders returns the standard valid user-scope auth headers for
// identity.
func userHeaders(identity string) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + testUserToken,
		"X-Identity-Id": identity,
		"Content-Type":  "application/json",
	}
}

func consumerHeaders() map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + testConsumerToken,
	}
}

// seedFeedback writes identity's feedback for modelKey directly through the
// concrete *sqlite.Store (bypassing the HTTP layer and feedback.Service),
// with an explicit, caller-chosen updated_at/created_at timestamp — the
// only way to get deterministic control over freshness/staleness
// boundaries (contract §6) without depending on internal/feedback.Service's
// own unexported clock, which this package cannot reach.
func (env *testEnv) seedFeedback(t *testing.T, identity feedback.IdentityID, modelKey feedback.ModelKey, overall int, skills []feedback.SkillRating, at time.Time) {
	t.Helper()
	input, err := feedback.NewFeedbackInput(string(modelKey), overall, skills, "")
	if err != nil {
		t.Fatalf("NewFeedbackInput: %v", err)
	}
	if _, err := env.store.UpsertFeedback(context.Background(), identity, input, at); err != nil {
		t.Fatalf("UpsertFeedback: %v", err)
	}
}
