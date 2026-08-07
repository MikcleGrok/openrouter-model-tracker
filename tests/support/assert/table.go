package assert

import (
	"strings"
	"testing"
)

type TableExpected struct {
	Rows []string
}

func TableRows(t *testing.T, output string, expected TableExpected) {
	t.Helper()
	for _, row := range expected.Rows {
		if !strings.Contains(output, row) {
			t.Errorf("table output does not contain %q:\n%s", row, output)
		}
	}
}
