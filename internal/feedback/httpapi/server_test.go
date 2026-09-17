package httpapi

import (
	"context"
	"io"
	"path/filepath"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/sqlite"
)

func newTestStore(t *testing.T) *sqlite.Store {
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
	return store
}

func TestNew_RejectsEmptyUserToken(t *testing.T) {
	store := newTestStore(t)
	_, err := New(store, Config{UserToken: nil, ConsumerToken: []byte(testConsumerToken)})
	if err == nil {
		t.Fatal("New with empty UserToken: want an error, got nil")
	}
}

// TestNew_RejectsIdenticalUserAndConsumerTokens is review round 1 finding
// #3: plan §6.1/§6.3 state the two credentials are "полностью отделён" as
// an explicit invariant. Without this check, an operator pointing
// --token-file and --consumer-token-file at the same secret gets a
// consumer endpoint that classifies every presentation of that shared
// secret as the *user* scope (classifyToken checks the user secret first)
// and therefore permanently 403s a legitimate consumer credential holder,
// with no diagnostic anywhere. New must refuse this configuration outright.
func TestNew_RejectsIdenticalUserAndConsumerTokens(t *testing.T) {
	store := newTestStore(t)
	shared := []byte(testUserToken)
	_, err := New(store, Config{UserToken: shared, ConsumerToken: shared})
	if err == nil {
		t.Fatal("New with identical user/consumer tokens: want an error, got nil")
	}
}

// TestNew_RejectsIdenticalTokens_DifferentSliceInstances proves the check
// compares byte content, not slice identity — a copied secret (the exact
// scenario the finding describes: "a copied secret") must still be caught.
func TestNew_RejectsIdenticalTokens_DifferentSliceInstances(t *testing.T) {
	store := newTestStore(t)
	userToken := []byte(testUserToken)
	consumerTokenCopy := append([]byte(nil), userToken...)
	_, err := New(store, Config{UserToken: userToken, ConsumerToken: consumerTokenCopy})
	if err == nil {
		t.Fatal("New with a byte-identical but distinct consumer token slice: want an error, got nil")
	}
}

func TestNew_AllowsDistinctTokens(t *testing.T) {
	store := newTestStore(t)
	s, err := New(store, Config{UserToken: []byte(testUserToken), ConsumerToken: []byte(testConsumerToken)})
	if err != nil {
		t.Fatalf("New with distinct tokens: unexpected error: %v", err)
	}
	if s == nil {
		t.Fatal("New returned a nil Server with a nil error")
	}
}

// TestNew_AllowsNilConsumerToken confirms an absent consumer token (the
// consumer endpoint effectively disabled) is not itself an error — only an
// actually-*matching* pair is rejected.
func TestNew_AllowsNilConsumerToken(t *testing.T) {
	store := newTestStore(t)
	if _, err := New(store, Config{UserToken: []byte(testUserToken)}); err != nil {
		t.Fatalf("New with nil ConsumerToken: unexpected error: %v", err)
	}
}
