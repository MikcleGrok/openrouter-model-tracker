package client_test

import (
	"context"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	client "github.com/sboborikin/openrouter-model-tracker/internal/feedback/client"
	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/httpapi"
	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/sqlite"
)

// This file proves internal/feedback/client against a REAL, shipped
// feedback server — a real *sqlite.Store + real httpapi.Server behind an
// httptest.Server — not merely this package's own hand-written JSON
// fixtures (client_test.go), mirroring internal/feedback/consumer's own
// contract_test.go pattern (newRealServerEnv). Every other test in this
// package answers "does this client parse the JSON shape *I* wrote
// correctly"; this one answers "does this client parse what the real
// server actually sends" — closing the gap where a field rename in
// httpapi/dto.go could leave every hand-fixture test green while the TUI
// silently renders wrong or zero-valued data.
//
// It lives in an external "client_test" package (not "client") precisely so
// internal/feedback/sqlite — and the SQLite driver it pulls in
// (modernc.org/sqlite) — is a dependency of THIS TEST FILE ONLY, never of
// the client package's own non-test import graph: doc.go's "Deliberately
// decoupled" section is exactly what guarantees the TUI binary
// (cmd/openrouter, which imports internal/feedback/client) never links a
// SQLite driver. `go list -deps ./cmd/openrouter` confirms zero
// modernc.org/sqlite both before and after this file exists, since a
// _test.go file — internal or external package — is never compiled into a
// non-test binary in the first place; the external package is the belt to
// that suspenders, keeping the dependency out of `go list -deps
// ./internal/feedback/client`'s own (test-inclusive) graph too.
const roundTripUserToken = "roundtrip-user-secret-aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func TestClient_RoundTripAgainstRealServer_PutThenGetSummary(t *testing.T) {
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

	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	server, err := httpapi.New(store, httpapi.Config{
		UserToken: []byte(roundTripUserToken),
		BackupDir: filepath.Join(t.TempDir(), "backups"),
		Now:       func() time.Time { return now },
	})
	if err != nil {
		t.Fatalf("httpapi.New: %v", err)
	}
	srv := httptest.NewServer(server.Handler())
	t.Cleanup(srv.Close)

	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")
	if err := os.WriteFile(tokenFile, []byte(roundTripUserToken+"\n"), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	if _, _, err := client.EnsureIdentityFile(identityFile); err != nil {
		t.Fatalf("EnsureIdentityFile: %v", err)
	}

	c, err := client.New(client.Config{
		Endpoint:       srv.URL,
		TokenFile:      tokenFile,
		IdentityFile:   identityFile,
		RequestTimeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("client.New: %v", err)
	}

	const modelKey = "acme/roundtrip-model"
	putReq := client.FeedbackRequest{
		Overall: 4,
		Skills:  []client.SkillRating{{Key: "coding", Rating: 5}},
		Review:  "solid model",
	}
	putGot, err := c.PutFeedback(ctx, modelKey, putReq)
	if err != nil {
		t.Fatalf("PutFeedback: %v", err)
	}
	if putGot.Mine == nil || putGot.Mine.Overall != 4 || putGot.Mine.Review != "solid model" {
		t.Fatalf("PutFeedback response Mine = %+v, want overall=4 review=%q", putGot.Mine, "solid model")
	}
	if len(putGot.Mine.Skills) != 1 || putGot.Mine.Skills[0].Key != "coding" || putGot.Mine.Skills[0].Rating != 5 {
		t.Fatalf("PutFeedback response Mine.Skills = %+v, want [{coding 5}]", putGot.Mine.Skills)
	}
	if putGot.Community == nil || putGot.Community.Count != 1 || putGot.Community.Average == nil || *putGot.Community.Average != 4 {
		t.Fatalf("PutFeedback response Community = %+v, want count=1 average=4", putGot.Community)
	}
	if putGot.PersonalPosition.Status != client.PositionRanked || putGot.PersonalPosition.Value != 1 {
		t.Fatalf("PutFeedback response PersonalPosition = %+v, want ranked/1", putGot.PersonalPosition)
	}

	// A separate GetSummary call must see exactly what PutFeedback just
	// wrote — this is the real cross-request round trip the hand-fixture
	// tests can never exercise (they answer each call from a static
	// literal, never from server state written by an earlier call).
	summary, err := c.GetSummary(ctx, modelKey, false)
	if err != nil {
		t.Fatalf("GetSummary: %v", err)
	}
	if summary.ModelKey != modelKey {
		t.Errorf("GetSummary ModelKey = %q, want %q", summary.ModelKey, modelKey)
	}
	if summary.Mine == nil || summary.Mine.Overall != 4 || summary.Mine.Review != "solid model" {
		t.Fatalf("GetSummary Mine = %+v, want overall=4 review=%q (the exact PUT just sent)", summary.Mine, "solid model")
	}
	if len(summary.Mine.Skills) != 1 || summary.Mine.Skills[0].Key != "coding" || summary.Mine.Skills[0].Rating != 5 {
		t.Fatalf("GetSummary Mine.Skills = %+v, want [{coding 5}]", summary.Mine.Skills)
	}
	if summary.Community == nil || summary.Community.Count != 1 || summary.Community.Average == nil || *summary.Community.Average != 4 {
		t.Fatalf("GetSummary Community = %+v, want count=1 average=4", summary.Community)
	}
	if summary.PersonalPosition.Status != client.PositionRanked || summary.PersonalPosition.Value != 1 {
		t.Fatalf("GetSummary PersonalPosition = %+v, want ranked/1", summary.PersonalPosition)
	}
	if summary.BasePosition.Status != client.PositionUnranked {
		t.Errorf("GetSummary BasePosition.Status = %q, want unranked", summary.BasePosition.Status)
	}

	own, err := c.GetOwnFeedback(ctx, modelKey)
	if err != nil {
		t.Fatalf("GetOwnFeedback: %v", err)
	}
	if own.OwnFeedback == nil || own.OwnFeedback.Overall != 4 || own.OwnFeedback.Review != "solid model" {
		t.Fatalf("GetOwnFeedback = %+v, want overall=4 review=%q", own.OwnFeedback, "solid model")
	}
}
