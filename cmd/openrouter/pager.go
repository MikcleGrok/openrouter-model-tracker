package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"

	"golang.org/x/term"
)

var pagerIsTTY = func(stdout io.Writer) bool {
	file, ok := stdout.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(file.Fd()))
}

func shouldPage(stdout io.Writer, noPager bool) bool {
	return !noPager && pagerIsTTY(stdout)
}

var runPager = func(output string, stdout, stderr io.Writer) error {
	pager := exec.Command("less", "-S")
	pager.Stdin = strings.NewReader(output)
	pager.Stdout = stdout
	pager.Stderr = stderr
	if err := pager.Run(); err != nil {
		return fmt.Errorf("run less -S: %w", err)
	}
	return nil
}

func writePagedOutput(output string, stdout, stderr io.Writer, page bool) error {
	if page {
		return runPager(output, stdout, stderr)
	}
	_, err := io.WriteString(stdout, output)
	return err
}
