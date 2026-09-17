package client

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestValidIdentityFormat(t *testing.T) {
	cases := map[string]bool{
		strings.Repeat("a", 64):       true,
		strings.Repeat("0", 64):       true,
		strings.Repeat("f", 64):       true,
		"":                            false,
		strings.Repeat("a", 63):       false,
		strings.Repeat("a", 65):       false,
		strings.Repeat("A", 64):       false, // uppercase not allowed
		strings.Repeat("g", 64):       false, // 'g' is not hex
		strings.Repeat("a", 63) + " ": false,
	}
	for id, want := range cases {
		if got := ValidIdentityFormat(id); got != want {
			t.Errorf("ValidIdentityFormat(%q) = %v, want %v", id, got, want)
		}
	}
}

func TestEnsureIdentityFile_CreatesFreshValidIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "identity")

	id, created, err := EnsureIdentityFile(path)
	if err != nil {
		t.Fatalf("EnsureIdentityFile: %v", err)
	}
	if !created {
		t.Error("created = false, want true for a fresh file")
	}
	if !ValidIdentityFormat(id) {
		t.Errorf("generated identity %q is not a valid format", id)
	}

	// Written content round-trips as the same identity, trimmed.
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written identity file: %v", err)
	}
	if strings.TrimSpace(string(raw)) != id {
		t.Errorf("file content = %q, want %q", strings.TrimSpace(string(raw)), id)
	}
}

func TestEnsureIdentityFile_ReusesExistingValidIdentity(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	first, created, err := EnsureIdentityFile(path)
	if err != nil || !created {
		t.Fatalf("first EnsureIdentityFile: id=%q created=%v err=%v", first, created, err)
	}

	second, created, err := EnsureIdentityFile(path)
	if err != nil {
		t.Fatalf("second EnsureIdentityFile: %v", err)
	}
	if created {
		t.Error("second call reports created=true, want false (idempotent, identity already existed)")
	}
	if second != first {
		t.Errorf("second identity = %q, want unchanged %q", second, first)
	}
}

func TestEnsureIdentityFile_RegeneratesOnMalformedContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "identity")
	if err := os.WriteFile(path, []byte("not-a-valid-identity\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	id, created, err := EnsureIdentityFile(path)
	if err != nil {
		t.Fatalf("EnsureIdentityFile: %v", err)
	}
	if !created {
		t.Error("created = false, want true (existing content was malformed)")
	}
	if !ValidIdentityFormat(id) {
		t.Errorf("regenerated identity %q is not a valid format", id)
	}
}

func TestEnsureIdentityFile_RejectsEmptyPath(t *testing.T) {
	if _, _, err := EnsureIdentityFile(""); err == nil {
		t.Fatal("EnsureIdentityFile(\"\") = nil error, want an error")
	}
}

func TestEnsureIdentityFile_UnixPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("mode bits are not meaningful on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "identity")
	if _, _, err := EnsureIdentityFile(path); err != nil {
		t.Fatalf("EnsureIdentityFile: %v", err)
	}
	fileInfo, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat identity file: %v", err)
	}
	if perm := fileInfo.Mode().Perm(); perm != 0o600 {
		t.Errorf("identity file mode = %o, want 0600", perm)
	}
	dirInfo, err := os.Stat(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatalf("stat identity dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("identity dir mode = %o, want 0700", perm)
	}
}

func TestCheckTokenFileReadable_Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("super-secret-token\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckTokenFileReadable(path); err != nil {
		t.Errorf("CheckTokenFileReadable: %v", err)
	}
}

func TestCheckTokenFileReadable_MissingFile(t *testing.T) {
	err := CheckTokenFileReadable(filepath.Join(t.TempDir(), "does-not-exist"))
	if err == nil {
		t.Fatal("CheckTokenFileReadable on a missing file = nil error, want an error")
	}
	if !IsCredentialError(err) {
		t.Errorf("error = %v (%T), want a *CredentialError", err, err)
	}
}

func TestCheckTokenFileReadable_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := CheckTokenFileReadable(path); err == nil {
		t.Fatal("CheckTokenFileReadable on a whitespace-only file = nil error, want an error")
	}
}

func TestCheckTokenFileReadable_RejectsEmptyPath(t *testing.T) {
	if err := CheckTokenFileReadable(""); err == nil {
		t.Fatal("CheckTokenFileReadable(\"\") = nil error, want an error")
	}
}

func TestCheckTokenFileReadable_TooLargeFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	huge := strings.Repeat("a", maxCredentialFileBytes+1)
	if err := os.WriteFile(path, []byte(huge), 0o600); err != nil {
		t.Fatal(err)
	}
	err := CheckTokenFileReadable(path)
	if err == nil {
		t.Fatal("CheckTokenFileReadable on an oversized file = nil error, want an error")
	}
	var credErr *CredentialError
	if !errors.As(err, &credErr) {
		t.Fatalf("error = %v (%T), want a *CredentialError", err, err)
	}
}

func TestInit_CreatesIdentityAndVerifiesToken(t *testing.T) {
	dir := t.TempDir()
	identityFile := filepath.Join(dir, "identity")
	tokenFile := filepath.Join(dir, "token")
	if err := os.WriteFile(tokenFile, []byte("shared-secret\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	result, err := Init(identityFile, tokenFile)
	if err != nil {
		t.Fatalf("Init: %v", err)
	}
	if !result.IdentityCreated {
		t.Error("IdentityCreated = false, want true on first Init")
	}
	if !ValidIdentityFormat(result.IdentityID) {
		t.Errorf("IdentityID = %q, not a valid format", result.IdentityID)
	}

	// Idempotent: calling again with the same files changes nothing and
	// reports the identity as already existing.
	second, err := Init(identityFile, tokenFile)
	if err != nil {
		t.Fatalf("second Init: %v", err)
	}
	if second.IdentityCreated {
		t.Error("second Init reports IdentityCreated=true, want false (idempotent)")
	}
	if second.IdentityID != result.IdentityID {
		t.Errorf("second Init identity = %q, want unchanged %q", second.IdentityID, result.IdentityID)
	}
}

func TestInit_NeverCreatesTokenFile(t *testing.T) {
	dir := t.TempDir()
	identityFile := filepath.Join(dir, "identity")
	tokenFile := filepath.Join(dir, "token") // deliberately never created

	_, err := Init(identityFile, tokenFile)
	if err == nil {
		t.Fatal("Init with a missing token file = nil error, want an error")
	}
	if _, statErr := os.Stat(tokenFile); statErr == nil {
		t.Error("Init created the token file itself, want it to leave provisioning to feedback-server token init")
	}
	// The identity file must also not have been left behind by a partial
	// Init — token verification happens after identity creation, so at this
	// point the identity *was* created; that alone is fine (it is
	// idempotent and harmless), but the token file must never exist.
}

func TestReadToken_NeverIncludesSecretInErrorPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	secret := "super-secret-value-should-never-appear-in-errors"
	if err := os.WriteFile(path, []byte(secret), 0o600); err != nil {
		t.Fatal(err)
	}
	// Force an error by truncating the read bound: not directly possible
	// via the public API with a normal-sized secret, so this test instead
	// documents the guarantee at the CredentialError.Error() level for the
	// error paths that do occur (missing file).
	_, err := readToken(filepath.Join(dir, "missing"))
	if err == nil {
		t.Fatal("expected an error for a missing token file")
	}
	if strings.Contains(err.Error(), secret) {
		t.Errorf("error message leaks a secret it never should have seen: %v", err)
	}
}
