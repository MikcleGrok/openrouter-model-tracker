package arrange

import (
	"fmt"
	"strings"
	"testing"
	"time"
)

// UniqueID creates a readable marker that is unique for the current test.
func UniqueID(t *testing.T, prefix string) string {
	t.Helper()
	clean := strings.NewReplacer("/", "-", " ", "-").Replace(prefix)
	return fmt.Sprintf("%s-%d", clean, time.Now().UnixNano())
}
