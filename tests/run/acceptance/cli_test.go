package acceptance

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
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

func TestE2E_ScoreSource(t *testing.T) {
	t.Parallel()
	marker := arrange.UniqueID(t, "score-source")
	dataDir := arrange.ScoreSourceDataDir(t, marker)
	config := arrange.Config(t, dataDir)

	swe := act.Run(t, binary(t), "table", "--config", config, "--no-pager", "--slug")
	assert.Success(t, swe)
	assert.Contains(t, swe, "70.0%")
	if strings.Contains(swe.Stdout, "Elo") {
		t.Errorf("the default swebench view leaked an Arena number:\n%s", swe.Stdout)
	}

	arena := act.Run(t, binary(t), "table", "--config", config, "--no-pager", "--slug", "--score-source=arena")
	assert.Success(t, arena)
	assert.Contains(t, arena, "1453 Elo", "Score source: arena")
	if strings.Contains(arena.Stdout, "70.0%") {
		t.Errorf("the arena view leaked a SWE-bench number:\n%s", arena.Stdout)
	}

	bad := act.Run(t, binary(t), "table", "--config", config, "--no-pager", "--score-source=auto")
	assert.Failure(t, bad, "invalid --score-source")
}

func TestE2E_Check(t *testing.T) {
	t.Parallel()
	marker := arrange.UniqueID(t, "check")
	dataDir := arrange.DataDir(t, marker)
	config := arrange.Config(t, dataDir)
	output := filepath.Join(dataDir, "output.md")
	before := assert.Unchanged(t, filepath.Join(dataDir, "model-snapshot.json"))
	result := act.Run(t, binary(t), "check", "--config", config, "--output", output)
	assert.Success(t, result)
	assert.StillUnchanged(t, before)
	assert.Missing(t, output)
}

// tableBreakingLine matches a bare line that starts a GFM table cell without
// a leading "|" — the exact shape of the bug this feature fixes (the old
// template's "  Provenance: ...").
var tableBreakingLine = regexp.MustCompile(`(?m)^  Provenance:`)

func TestE2E_Report(t *testing.T) {
	t.Parallel()
	marker := arrange.UniqueID(t, "report")
	dataDir := arrange.DataDir(t, marker)
	config := arrange.Config(t, dataDir)
	output := filepath.Join(dataDir, "output.md")

	result := act.Run(t, binary(t), "report", "--config", config, "--output", output)
	assert.Success(t, result)

	body, err := os.ReadFile(output)
	if err != nil {
		t.Fatalf("read output: %v", err)
	}
	doc := string(body)
	if !strings.Contains(doc, "## Приложение") {
		t.Fatalf("report output has no provenance appendix:\n%s", doc)
	}
	if tableBreakingLine.MatchString(doc) {
		t.Fatalf("report output still has a table-breaking bare Provenance line:\n%s", doc)
	}
}

func TestE2E_ReportHTML(t *testing.T) {
	t.Parallel()
	marker := arrange.UniqueID(t, "report-html")
	dataDir := arrange.DataDir(t, marker)
	config := arrange.Config(t, dataDir)
	mdOutput := filepath.Join(dataDir, "out.md")
	htmlOutput := filepath.Join(dataDir, "out.html")

	result := act.Run(t, binary(t), "report", "--format", "both", "--config", config, "--output", mdOutput)
	assert.Success(t, result)

	if _, err := os.Stat(mdOutput); err != nil {
		t.Fatalf("markdown output missing: %v", err)
	}
	htmlBody, err := os.ReadFile(htmlOutput)
	if err != nil {
		t.Fatalf("read html output: %v", err)
	}
	html := string(htmlBody)
	if !strings.Contains(html, `id="ranked"`) {
		t.Fatalf("html output has no id=\"ranked\" section:\n%s", html)
	}
	if !strings.Contains(html, `class="sortable"`) {
		t.Fatalf("html output has no sortable table:\n%s", html)
	}
	if strings.Contains(html, "[vals.ai](") {
		t.Fatalf("html output leaked a Markdown link:\n%s", html)
	}
}

func TestE2E_InvalidCommandWritesErrorToStderr(t *testing.T) {
	t.Parallel()
	result := act.Run(t, binary(t), "not-a-command")
	assert.Failure(t, result, "unknown command")
}
