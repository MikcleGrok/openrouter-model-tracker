package act

import (
	"bytes"
	"os/exec"
	"testing"
)

type Result struct {
	Stdout string
	Stderr string
	Err    error
}

func Run(t *testing.T, binary string, args ...string) Result {
	t.Helper()
	cmd := exec.Command(binary, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return Result{Stdout: stdout.String(), Stderr: stderr.String(), Err: err}
}
