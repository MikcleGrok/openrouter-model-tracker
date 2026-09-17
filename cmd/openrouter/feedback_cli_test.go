package main

import (
	"bytes"
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
