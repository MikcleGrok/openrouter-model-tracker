package clipboard

import (
	"bytes"
	"errors"
	"testing"
)

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("terminal unavailable") }

func TestSynchronizedWritesWholePayloadAndPropagatesFailure(t *testing.T) {
	var output bytes.Buffer
	w := NewSynchronized(&output)
	if _, err := w.Write([]byte("frame")); err != nil {
		t.Fatal(err)
	}
	if output.String() != "frame" {
		t.Fatalf("output = %q", output.String())
	}
	if _, err := (&Synchronized{out: failWriter{}}).Write([]byte("x")); err == nil {
		t.Fatal("expected writer failure")
	}
}

func TestWriteFallsBackToOSC52AndReportsFallbackFailure(t *testing.T) {
	var output bytes.Buffer
	if err := Write(func(string) error { return errors.New("native unavailable") }, "copy", &output); err != nil {
		t.Fatalf("fallback write failed: %v", err)
	}
	if output.Len() == 0 {
		t.Fatal("fallback produced no terminal protocol")
	}
	if err := Write(func(string) error { return errors.New("native unavailable") }, "copy", failWriter{}); err == nil {
		t.Fatal("expected fallback failure")
	}
}
