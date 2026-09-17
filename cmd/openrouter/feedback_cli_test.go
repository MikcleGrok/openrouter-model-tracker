package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	feedbackclient "github.com/sboborikin/openrouter-model-tracker/internal/feedback/client"
)

func writeFeedbackConfig(t *testing.T, tokenFile, identityFile string, enabled bool) string {
	t.Helper()
	body := "feedback:\n" +
		"  enabled: " + map[bool]string{true: "true", false: "false"}[enabled] + "\n" +
		"  token_file: " + tokenFile + "\n" +
		"  identity_file: " + identityFile + "\n"
	return writeConfig(t, body)
}

// writeFeedbackConfigWithEndpoint is writeFeedbackConfig plus an explicit
// endpoint, for tests (feedback delete) that need the config to actually
// point at a real (httptest) server rather than the default.
func writeFeedbackConfigWithEndpoint(t *testing.T, tokenFile, identityFile, endpoint string) string {
	t.Helper()
	body := "feedback:\n" +
		"  enabled: true\n" +
		"  endpoint: " + endpoint + "\n" +
		"  token_file: " + tokenFile + "\n" +
		"  identity_file: " + identityFile + "\n"
	return writeConfig(t, body)
}

// writeFeedbackIdentityCreds writes a token file and a valid identity file
// for tests that need runFeedbackDelete to actually build a client (the
// identity file must already hold a syntactically valid identity — unlike
// runFeedbackInit, delete never creates one).
func writeFeedbackIdentityCreds(t *testing.T, dir string) (tokenFile, identityFile string) {
	t.Helper()
	tokenFile = filepath.Join(dir, "token")
	identityFile = filepath.Join(dir, "identity")
	if err := os.WriteFile(tokenFile, []byte("shared-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := feedbackclient.EnsureIdentityFile(identityFile); err != nil {
		t.Fatalf("EnsureIdentityFile: %v", err)
	}
	return tokenFile, identityFile
}

func TestRunFeedbackInit_CreatesIdentityAndVerifiesToken(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")
	if err := os.WriteFile(tokenFile, []byte("shared-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := writeFeedbackConfig(t, tokenFile, identityFile, true)

	var out bytes.Buffer
	if err := runFeedbackInit(configPath, &out); err != nil {
		t.Fatalf("runFeedbackInit: %v", err)
	}
	if !strings.Contains(out.String(), "Created: "+identityFile) {
		t.Errorf("output %q does not report identity creation", out.String())
	}
	if !strings.Contains(out.String(), "Token file OK: "+tokenFile) {
		t.Errorf("output %q does not confirm the token file", out.String())
	}
	if strings.Contains(out.String(), "Note: feedback.enabled is false") {
		t.Errorf("output %q should not warn about a disabled feature when enabled: true", out.String())
	}

	raw, err := os.ReadFile(identityFile)
	if err != nil {
		t.Fatalf("read identity file: %v", err)
	}
	id := strings.TrimSpace(string(raw))
	if !feedbackclient.ValidIdentityFormat(id) {
		t.Errorf("identity file content %q is not a valid identity", id)
	}
	if !strings.Contains(out.String(), id) {
		t.Errorf("output %q does not mention the created identity %q", out.String(), id)
	}
}

func TestRunFeedbackInit_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")
	if err := os.WriteFile(tokenFile, []byte("shared-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := writeFeedbackConfig(t, tokenFile, identityFile, true)

	var first bytes.Buffer
	if err := runFeedbackInit(configPath, &first); err != nil {
		t.Fatalf("first runFeedbackInit: %v", err)
	}
	firstID, err := os.ReadFile(identityFile)
	if err != nil {
		t.Fatal(err)
	}

	var second bytes.Buffer
	if err := runFeedbackInit(configPath, &second); err != nil {
		t.Fatalf("second runFeedbackInit: %v", err)
	}
	if !strings.Contains(second.String(), "Already exists: "+identityFile) {
		t.Errorf("second output %q does not report the identity as already existing", second.String())
	}
	secondID, err := os.ReadFile(identityFile)
	if err != nil {
		t.Fatal(err)
	}
	if string(firstID) != string(secondID) {
		t.Errorf("identity changed across idempotent init calls: %q vs %q", firstID, secondID)
	}
}

func TestRunFeedbackInit_ReportsDisabledFeature(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")
	if err := os.WriteFile(tokenFile, []byte("shared-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := writeFeedbackConfig(t, tokenFile, identityFile, false)

	var out bytes.Buffer
	if err := runFeedbackInit(configPath, &out); err != nil {
		t.Fatalf("runFeedbackInit: %v", err)
	}
	if !strings.Contains(out.String(), "Note: feedback.enabled is false") {
		t.Errorf("output %q does not note that feedback.enabled is false", out.String())
	}
}

func TestRunFeedbackInit_MissingTokenFileFails(t *testing.T) {
	dir := t.TempDir()
	identityFile := filepath.Join(dir, "identity")
	configPath := writeFeedbackConfig(t, filepath.Join(dir, "missing-token"), identityFile, true)

	var out bytes.Buffer
	err := runFeedbackInit(configPath, &out)
	if err == nil {
		t.Fatal("runFeedbackInit with a missing token file: want error, got nil")
	}
	if strings.Contains(err.Error(), "shared-secret") {
		t.Errorf("error %q must never echo token content", err.Error())
	}
	// runFeedbackInit never reports success output once the token check
	// fails (Init returns before this function prints anything), even
	// though the identity file itself was already created as a side effect
	// of feedbackclient.Init running EnsureIdentityFile before
	// CheckTokenFileReadable — a subsequent init call with a fixed token
	// file simply reuses that identity rather than creating a new one.
	if strings.Contains(out.String(), "Created:") || strings.Contains(out.String(), "Token file OK") {
		t.Errorf("output %q should not report success when the token file check failed", out.String())
	}
}

func TestRunFeedbackInit_UsesConfigRelativePaths(t *testing.T) {
	root := t.TempDir()
	configPath := filepath.Join(root, "nested", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "nested", "token"), []byte("secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	body := "feedback:\n  enabled: true\n  token_file: token\n  identity_file: identity\ndefault_filter: \"\"\n"
	if err := os.WriteFile(configPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	if err := runFeedbackInit(configPath, &out); err != nil {
		t.Fatalf("runFeedbackInit: %v", err)
	}
	wantIdentity := filepath.Join(root, "nested", "identity")
	if _, statErr := os.Stat(wantIdentity); statErr != nil {
		t.Fatalf("expected identity file at %s (resolved relative to the config file): %v", wantIdentity, statErr)
	}
}

func TestFeedbackInitCommandEndToEnd(t *testing.T) {
	dir := t.TempDir()
	tokenFile := filepath.Join(dir, "token")
	identityFile := filepath.Join(dir, "identity")
	if err := os.WriteFile(tokenFile, []byte("shared-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	configPath := writeFeedbackConfig(t, tokenFile, identityFile, true)

	output := executeCLI(t, "feedback", "init", "--config", configPath)
	if !strings.Contains(output, "Created: "+identityFile) {
		t.Fatalf("feedback init output = %q", output)
	}
	if _, err := os.Stat(identityFile); err != nil {
		t.Fatalf("identity file was not created: %v", err)
	}
}

func TestFeedbackInitHelpIsEnglish(t *testing.T) {
	output := executeCLI(t, "feedback", "init", "--help")
	for _, want := range []string{"Create the local feedback identity", "feedback-server token init", "verify the token file is readable"} {
		if !strings.Contains(output, want) {
			t.Errorf("feedback init help does not contain %q:\n%s", want, output)
		}
	}
}

// --- feedback delete ---

// TestRunFeedbackDelete_204ReportsDone covers the fully-done outcome, and
// confirms the real DELETE /v1/me/feedback request is actually sent.
func TestRunFeedbackDelete_204ReportsDone(t *testing.T) {
	dir := t.TempDir()
	tokenFile, identityFile := writeFeedbackIdentityCreds(t, dir)

	var gotMethod, gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotPath = r.Method, r.URL.Path
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	configPath := writeFeedbackConfigWithEndpoint(t, tokenFile, identityFile, srv.URL)

	var out bytes.Buffer
	if err := runFeedbackDelete(configPath, true, strings.NewReader(""), &out); err != nil {
		t.Fatalf("runFeedbackDelete: %v", err)
	}
	if gotMethod != http.MethodDelete || gotPath != "/v1/me/feedback" {
		t.Errorf("method/path = %s %s, want DELETE /v1/me/feedback", gotMethod, gotPath)
	}
	if !strings.Contains(out.String(), "Deleted") {
		t.Errorf("output %q does not confirm the delete completed", out.String())
	}
}

// TestRunFeedbackDelete_202ReportsPendingNotFailure covers the
// committed-but-cleanup-still-finishing outcome: it must be reported as a
// success, never as an error, and must say re-running is safe.
func TestRunFeedbackDelete_202ReportsPendingNotFailure(t *testing.T) {
	dir := t.TempDir()
	tokenFile, identityFile := writeFeedbackIdentityCreds(t, dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"status":"cleanup_pending"}`))
	}))
	defer srv.Close()
	configPath := writeFeedbackConfigWithEndpoint(t, tokenFile, identityFile, srv.URL)

	var out bytes.Buffer
	if err := runFeedbackDelete(configPath, true, strings.NewReader(""), &out); err != nil {
		t.Fatalf("runFeedbackDelete: %v, want nil error for a 202 cleanup_pending (it is a success)", err)
	}
	if !strings.Contains(out.String(), "safe to re-run") {
		t.Errorf("output %q does not say re-running is safe", out.String())
	}
}

// TestRunFeedbackDelete_ServerErrorFails covers the failure outcome: an
// error must be returned, and the output must never claim success.
func TestRunFeedbackDelete_ServerErrorFails(t *testing.T) {
	dir := t.TempDir()
	tokenFile, identityFile := writeFeedbackIdentityCreds(t, dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"error":"internal error"}`))
	}))
	defer srv.Close()
	configPath := writeFeedbackConfigWithEndpoint(t, tokenFile, identityFile, srv.URL)

	var out bytes.Buffer
	err := runFeedbackDelete(configPath, true, strings.NewReader(""), &out)
	if err == nil {
		t.Fatal("runFeedbackDelete with a 503 = nil error, want an error")
	}
	if strings.Contains(out.String(), "Deleted") || strings.Contains(out.String(), "safe to re-run") {
		t.Errorf("output %q must not claim success when the server call failed", out.String())
	}
}

// TestRunFeedbackDelete_NoConfirmationDoesNotDelete is the confirmation-gate
// regression test: without --yes and without an explicit "y"/"yes" answer,
// the server must never be contacted at all.
func TestRunFeedbackDelete_NoConfirmationDoesNotDelete(t *testing.T) {
	dir := t.TempDir()
	tokenFile, identityFile := writeFeedbackIdentityCreds(t, dir)

	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	configPath := writeFeedbackConfigWithEndpoint(t, tokenFile, identityFile, srv.URL)

	for _, answer := range []string{"n\n", "no\n", "\n", ""} {
		called = false
		var out bytes.Buffer
		if err := runFeedbackDelete(configPath, false, strings.NewReader(answer), &out); err != nil {
			t.Fatalf("runFeedbackDelete with answer %q: %v, want nil error (declining is not a failure)", answer, err)
		}
		if called {
			t.Fatalf("answer %q: DELETE request was sent despite no confirmation", answer)
		}
		if strings.Contains(out.String(), "Deleted") {
			t.Fatalf("answer %q: output %q claims success despite no confirmation", answer, out.String())
		}
		if !strings.Contains(out.String(), "Aborted") {
			t.Fatalf("answer %q: output %q does not report the delete as aborted", answer, out.String())
		}
	}
}

// TestRunFeedbackDelete_ExplicitYesConfirmationDeletes proves the prompt
// itself accepts an explicit "y"/"yes" typed on stdin (not just --yes) and
// that the request is then actually sent.
func TestRunFeedbackDelete_ExplicitYesConfirmationDeletes(t *testing.T) {
	dir := t.TempDir()
	tokenFile, identityFile := writeFeedbackIdentityCreds(t, dir)

	for _, answer := range []string{"y\n", "yes\n", "YES\n", "  y  \n"} {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			called = true
			w.WriteHeader(http.StatusNoContent)
		}))
		configPath := writeFeedbackConfigWithEndpoint(t, tokenFile, identityFile, srv.URL)

		var out bytes.Buffer
		if err := runFeedbackDelete(configPath, false, strings.NewReader(answer), &out); err != nil {
			t.Fatalf("answer %q: runFeedbackDelete: %v", answer, err)
		}
		if !called {
			t.Fatalf("answer %q: DELETE request was never sent despite an explicit yes", answer)
		}
		if !strings.Contains(out.String(), "Deleted") {
			t.Fatalf("answer %q: output %q does not confirm the delete", answer, out.String())
		}
		srv.Close()
	}
}

// TestRunFeedbackDelete_YesFlagSkipsPrompt proves skipConfirm=true (the
// --yes/-y flag) never prints or waits on the confirmation prompt at all.
func TestRunFeedbackDelete_YesFlagSkipsPrompt(t *testing.T) {
	dir := t.TempDir()
	tokenFile, identityFile := writeFeedbackIdentityCreds(t, dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	configPath := writeFeedbackConfigWithEndpoint(t, tokenFile, identityFile, srv.URL)

	var out bytes.Buffer
	// A reader that fails the test if it is ever read from: --yes must not
	// touch stdin at all.
	poisoned := &failingReader{t: t}
	if err := runFeedbackDelete(configPath, true, poisoned, &out); err != nil {
		t.Fatalf("runFeedbackDelete: %v", err)
	}
	if strings.Contains(out.String(), "Continue?") {
		t.Errorf("output %q shows the confirmation prompt despite --yes", out.String())
	}
	if !strings.Contains(out.String(), "Deleted") {
		t.Errorf("output %q does not confirm the delete", out.String())
	}
}

// failingReader fails the test on any Read call — used to prove a code path
// never touches stdin.
type failingReader struct{ t *testing.T }

func (r *failingReader) Read([]byte) (int, error) {
	r.t.Helper()
	r.t.Fatal("unexpected read from stdin")
	return 0, nil
}

// TestFeedbackDeleteCommandEndToEnd drives the real cobra command with
// --yes, proving the flag is wired through to skip the prompt end-to-end.
func TestFeedbackDeleteCommandEndToEnd(t *testing.T) {
	dir := t.TempDir()
	tokenFile, identityFile := writeFeedbackIdentityCreds(t, dir)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()
	configPath := writeFeedbackConfigWithEndpoint(t, tokenFile, identityFile, srv.URL)

	output := executeCLI(t, "feedback", "delete", "--yes", "--config", configPath)
	if !strings.Contains(output, "Deleted") {
		t.Fatalf("feedback delete output = %q", output)
	}
}

func TestFeedbackDeleteHelpIsEnglish(t *testing.T) {
	output := executeCLI(t, "feedback", "delete", "--help")
	for _, want := range []string{"Permanently delete", "DELETE /v1/me/feedback", "cannot be recovered", "--yes"} {
		if !strings.Contains(output, want) {
			t.Errorf("feedback delete help does not contain %q:\n%s", want, output)
		}
	}
}
