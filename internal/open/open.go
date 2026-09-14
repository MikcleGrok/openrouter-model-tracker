// Package open launches the host's default handler for a file — the same
// thing double-clicking it in a file manager would do. It never fails the
// caller's process: callers are expected to warn and continue, since by the
// time File is called the document it wants to open has already been
// written successfully.
package open

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// EnvOverride is the environment variable that overrides platform detection.
// Setting it to a binary name uses that binary as the opener instead of the
// platform default — the escape hatch for WSL (OPENROUTER_OPEN=wslview) and
// headless environments. Setting it to "0" disables opening outright: File
// returns ErrDisabled without spawning anything.
const EnvOverride = "OPENROUTER_OPEN"

// ErrDisabled is returned by File (via fileWith) when EnvOverride=0.
var ErrDisabled = errors.New("open: disabled by " + EnvOverride + "=0")

// Command returns the argv that opens path on goos, honouring override: a
// non-empty override always wins and becomes the opener binary, receiving
// path as its only argument. ok is false only when override is empty and
// goos has no known opener.
func Command(goos, override, path string) (name string, args []string, ok bool) {
	if override != "" {
		return override, []string{path}, true
	}
	switch goos {
	case "darwin":
		return "open", []string{path}, true
	case "linux", "freebsd", "openbsd", "netbsd":
		return "xdg-open", []string{path}, true
	case "windows":
		// The empty "" argument is load-bearing: `start` is a cmd builtin
		// that treats its first quoted argument as the new window's title,
		// so without a placeholder title any quoted path breaks.
		return "cmd", []string{"/c", "start", "", path}, true
	default:
		return "", nil, false
	}
}

// File opens path with the host's default handler, honouring the
// OPENROUTER_OPEN environment variable override.
func File(path string) error {
	return fileWith(runtime.GOOS, os.Getenv(EnvOverride), path, runCommand)
}

func runCommand(name string, args ...string) error {
	return exec.Command(name, args...).Start()
}

// fileWith is File's test seam: File = fileWith(runtime.GOOS,
// os.Getenv(EnvOverride), path, run).
func fileWith(goos, override, path string, run func(name string, args ...string) error) error {
	if override == "0" {
		return ErrDisabled
	}
	name, args, ok := Command(goos, override, path)
	if !ok {
		return fmt.Errorf("open: no known opener for this platform (GOOS=%s); set %s to override", goos, EnvOverride)
	}
	return run(name, args...)
}
