package acceptance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sboborikin/openrouter-model-tracker/tests/support/act"
	"github.com/sboborikin/openrouter-model-tracker/tests/support/arrange"
	"github.com/sboborikin/openrouter-model-tracker/tests/support/assert"
)

func binary(t *testing.T) string {
	t.Helper()
	return filepath.Join("..", "..", "..", "bin", "openrouter")
}

func TestE2E_Version(t *testing.T) {
	t.Parallel()
	expectedVersion := os.Getenv("OPENROUTER_EXPECTED_VERSION")
	if expectedVersion == "" {
		expectedVersion = "0.0.0-dev"
	}
	assert.Version(t, act.Run(t, binary(t), "--version"), expectedVersion)
}

func TestE2E_Help(t *testing.T) {
	t.Parallel()
	assert.Help(t, act.Run(t, binary(t), "--help"))
}

func TestE2E_Table(t *testing.T) {
	t.Parallel()
	marker := arrange.UniqueID(t, "table")
	dataDir := arrange.DataDir(t, marker)
	config := arrange.Config(t, dataDir)
	result := act.Run(t, binary(t), "table", "--config", config, "--no-pager", "--slug", "--task-fit", "long", "--limit", "1")
	assert.Table(t, result)
	assert.TableRows(t, result.Stdout, assert.TableExpected{Rows: []string{"demo/high", "implement + test"}})
}

func TestE2E_Check(t *testing.T) {
	t.Parallel()
	marker := arrange.UniqueID(t, "check")
	dataDir := arrange.DataDir(t, marker)
	config := arrange.Config(t, dataDir)
	output := filepath.Join(dataDir, "output.md")
	before := assert.Unchanged(t, filepath.Join(dataDir, "cache", "last-run-snapshot.json"))
	result := act.Run(t, binary(t), "check", "--config", config, "--output", output)
	assert.Success(t, result)
	assert.StillUnchanged(t, before)
	assert.Missing(t, output)
}

func TestE2E_InvalidCommandWritesErrorToStderr(t *testing.T) {
	t.Parallel()
	result := act.Run(t, binary(t), "not-a-command")
	assert.Failure(t, result, "unknown command")
}
