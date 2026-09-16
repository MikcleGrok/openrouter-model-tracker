package main

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestGenerateSecretHex_UniqueAndCorrectLength(t *testing.T) {
	a, err := generateSecretHex()
	if err != nil {
		t.Fatalf("generateSecretHex: %v", err)
	}
	b, err := generateSecretHex()
	if err != nil {
		t.Fatalf("generateSecretHex: %v", err)
	}
	if a == b {
		t.Fatalf("two calls returned the same secret: %q", a)
	}
	if len(a) != secretBytes*2 {
		t.Errorf("len(secret) = %d, want %d (hex-encoded)", len(a), secretBytes*2)
	}
}

func TestWriteSecretFileExclusive_CreatesWithCorrectPermissions(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits are not meaningful on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "token")

	if err := writeSecretFileExclusive(path, "the-secret"); err != nil {
		t.Fatalf("writeSecretFileExclusive: %v", err)
	}

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Errorf("file mode = %v, want 0600", perm)
	}

	dirInfo, err := os.Stat(filepath.Join(dir, "sub"))
	if err != nil {
		t.Fatalf("Stat dir: %v", err)
	}
	if perm := dirInfo.Mode().Perm(); perm != 0o700 {
		t.Errorf("directory mode = %v, want 0700", perm)
	}

	got, err := readSecretFile(path)
	if err != nil {
		t.Fatalf("readSecretFile: %v", err)
	}
	if string(got) != "the-secret" {
		t.Errorf("readSecretFile = %q, want %q", got, "the-secret")
	}
}

func TestWriteSecretFileExclusive_RefusesToOverwrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := writeSecretFileExclusive(path, "first"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	if err := writeSecretFileExclusive(path, "second"); err == nil {
		t.Fatal("second writeSecretFileExclusive: want an error, got nil")
	}
	got, err := readSecretFile(path)
	if err != nil {
		t.Fatalf("readSecretFile: %v", err)
	}
	if string(got) != "first" {
		t.Errorf("content after refused overwrite = %q, want unchanged %q", got, "first")
	}
}

func TestWriteSecretFileAtomic_ReplacesExistingContent(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "consumer-token")
	if err := writeSecretFileExclusive(path, "old-secret"); err != nil {
		t.Fatalf("initial write: %v", err)
	}

	if err := writeSecretFileAtomic(path, "new-secret"); err != nil {
		t.Fatalf("writeSecretFileAtomic: %v", err)
	}

	got, err := readSecretFile(path)
	if err != nil {
		t.Fatalf("readSecretFile: %v", err)
	}
	if string(got) != "new-secret" {
		t.Errorf("content after rotate = %q, want new-secret", got)
	}

	// No leftover temp files in the directory.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("directory entries = %v, want exactly one (the token file, no leftover temp file)", entries)
	}
}

func TestWriteSecretFileAtomic_WorksWithoutAPreexistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "token")
	if err := writeSecretFileAtomic(path, "fresh-secret"); err != nil {
		t.Fatalf("writeSecretFileAtomic on a new path: %v", err)
	}
	got, err := readSecretFile(path)
	if err != nil {
		t.Fatalf("readSecretFile: %v", err)
	}
	if string(got) != "fresh-secret" {
		t.Errorf("content = %q, want fresh-secret", got)
	}
}

func TestReadSecretFile_RejectsEmpty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty-token")
	if err := os.WriteFile(path, []byte("   \n"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := readSecretFile(path); err == nil {
		t.Error("readSecretFile on a blank file: want an error, got nil")
	}
}
