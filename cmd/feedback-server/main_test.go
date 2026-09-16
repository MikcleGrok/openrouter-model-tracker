package main

import (
	"bytes"
	"context"
	"testing"
)

// TestRootCmd_RequiresTokenFiles confirms --token-file/--consumer-token-file
// are enforced as required flags (plan 7.2) — running the server with
// neither set must fail fast, before doing anything with a database.
func TestRootCmd_RequiresTokenFiles(t *testing.T) {
	cmd := newRootCmd()
	cmd.SetArgs([]string{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.ExecuteContext(context.Background()); err == nil {
		t.Error("running with no --token-file/--consumer-token-file: want an error, got nil")
	}
}

// TestRootCmd_HasTokenAndConsumerTokenSubcommands confirms the CLI surface
// plan 6.1/6.3/7.2 requires: `token init` and `consumer-token init|rotate`.
func TestRootCmd_HasTokenAndConsumerTokenSubcommands(t *testing.T) {
	root := newRootCmd()

	tokenCmd, _, err := root.Find([]string{"token", "init"})
	if err != nil || tokenCmd.Name() != "init" {
		t.Errorf("root.Find([token init]) = %v, %v", tokenCmd, err)
	}

	consumerInit, _, err := root.Find([]string{"consumer-token", "init"})
	if err != nil || consumerInit.Name() != "init" {
		t.Errorf("root.Find([consumer-token init]) = %v, %v", consumerInit, err)
	}

	consumerRotate, _, err := root.Find([]string{"consumer-token", "rotate"})
	if err != nil || consumerRotate.Name() != "rotate" {
		t.Errorf("root.Find([consumer-token rotate]) = %v, %v", consumerRotate, err)
	}
}

func TestDefaultDBPathAndBackupDir_AreNonEmpty(t *testing.T) {
	if defaultDBPath() == "" {
		t.Error("defaultDBPath() is empty")
	}
	if defaultBackupDir() == "" {
		t.Error("defaultBackupDir() is empty")
	}
}
