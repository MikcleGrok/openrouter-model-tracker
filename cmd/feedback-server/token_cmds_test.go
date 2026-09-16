package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTokenInit_CreatesFileAndReportsPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "token")
	var out bytes.Buffer
	if err := runTokenInit(path, &out); err != nil {
		t.Fatalf("runTokenInit: %v", err)
	}
	if !strings.Contains(out.String(), path) {
		t.Errorf("output %q does not mention the created path %q", out.String(), path)
	}
	secret, err := readSecretFile(path)
	if err != nil {
		t.Fatalf("readSecretFile: %v", err)
	}
	if len(secret) == 0 {
		t.Error("created token file is empty")
	}
}

func TestRunConsumerTokenInitThenRotate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "consumer-token")
	var out bytes.Buffer

	if err := runConsumerTokenInit(path, &out); err != nil {
		t.Fatalf("runConsumerTokenInit: %v", err)
	}
	first, err := readSecretFile(path)
	if err != nil {
		t.Fatalf("readSecretFile after init: %v", err)
	}

	// A second init must refuse (provisioning is not rotation).
	if err := runConsumerTokenInit(path, &out); err == nil {
		t.Error("second runConsumerTokenInit: want an error (must not overwrite), got nil")
	}

	if err := runConsumerTokenRotate(path, &out); err != nil {
		t.Fatalf("runConsumerTokenRotate: %v", err)
	}
	second, err := readSecretFile(path)
	if err != nil {
		t.Fatalf("readSecretFile after rotate: %v", err)
	}
	if string(first) == string(second) {
		t.Error("rotate did not change the secret")
	}
}
