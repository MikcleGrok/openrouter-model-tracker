package builder

import (
	"os"
	"path/filepath"
	"testing"
)

type DataDirBuilder struct {
	modelMap string
	notes    string
}

func NewDataDir() *DataDirBuilder {
	return &DataDirBuilder{modelMap: "demo/high\ttier=sonnet\ndemo/low\ttier=haiku\n", notes: "models:\n  demo/high:\n    display: Demo High\n    task_fit: [implement, test]\n  demo/low:\n    display: Demo Low\n"}
}

func (b *DataDirBuilder) Write(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, body := range map[string]string{"model-map.tsv": b.modelMap, "notes.yaml": b.notes} {
		if err := os.WriteFile(filepath.Join(root, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	NewSnapshot().Write(t, filepath.Join(root, "model-snapshot.json"))
}
