package arrange

import (
	"os"
	"path/filepath"
	"testing"
)

// Config writes a config file whose paths are relative to its own directory.
func Config(t *testing.T, dataDir string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	body := []byte("data_dir: " + dataDir + "\ndefault_output: output.md\n")
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}
