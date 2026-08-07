package assert

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

func Unchanged(t *testing.T, paths ...string) map[string][32]byte {
	t.Helper()
	result := make(map[string][32]byte, len(paths))
	for _, path := range paths {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		result[path] = sha256.Sum256(body)
	}
	return result
}

func StillUnchanged(t *testing.T, before map[string][32]byte) {
	t.Helper()
	for path, want := range before {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		if got := sha256.Sum256(body); got != want {
			t.Errorf("file changed: %s", filepath.Clean(path))
		}
	}
}

func Missing(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Fatalf("unexpected file: %s", path)
	} else if !os.IsNotExist(err) {
		t.Fatalf("stat %s: %v", path, err)
	}
}

func Describe(path string) string {
	return fmt.Sprintf("file %s", filepath.Clean(path))
}
