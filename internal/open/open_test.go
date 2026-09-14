package open

import (
	"errors"
	"fmt"
	"testing"
)

func equalArgs(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestCommandDispatchesByGOOS(t *testing.T) {
	tests := []struct {
		goos     string
		wantName string
		wantArgs []string
		wantOK   bool
	}{
		{"darwin", "open", []string{"/tmp/x.md"}, true},
		{"linux", "xdg-open", []string{"/tmp/x.md"}, true},
		{"freebsd", "xdg-open", []string{"/tmp/x.md"}, true},
		{"openbsd", "xdg-open", []string{"/tmp/x.md"}, true},
		{"netbsd", "xdg-open", []string{"/tmp/x.md"}, true},
		{"windows", "cmd", []string{"/c", "start", "", "/tmp/x.md"}, true},
		{"plan9", "", nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.goos, func(t *testing.T) {
			name, args, ok := Command(tt.goos, "", "/tmp/x.md")
			if ok != tt.wantOK {
				t.Fatalf("Command(%q) ok = %v, want %v", tt.goos, ok, tt.wantOK)
			}
			if !ok {
				if name != "" || args != nil {
					t.Fatalf("Command(%q) with ok=false returned %q %v, want empty", tt.goos, name, args)
				}
				return
			}
			if name != tt.wantName || !equalArgs(args, tt.wantArgs) {
				t.Fatalf("Command(%q) = %q %v, want %q %v", tt.goos, name, args, tt.wantName, tt.wantArgs)
			}
		})
	}
}

func TestCommandOverrideWinsOverGOOSAndUnknownPlatform(t *testing.T) {
	for _, goos := range []string{"darwin", "windows", "plan9"} {
		name, args, ok := Command(goos, "custom-opener", "/tmp/x.md")
		if !ok || name != "custom-opener" || !equalArgs(args, []string{"/tmp/x.md"}) {
			t.Fatalf("Command(%q, custom-opener) = %q %v %v, want custom-opener [/tmp/x.md] true", goos, name, args, ok)
		}
	}
}

func TestFileWithDisabledByEnvOverrideZero(t *testing.T) {
	called := false
	err := fileWith("darwin", "0", "/tmp/x.md", func(string, ...string) error {
		called = true
		return nil
	})
	if !errors.Is(err, ErrDisabled) {
		t.Fatalf("fileWith override=0 err = %v, want ErrDisabled", err)
	}
	if called {
		t.Fatal("fileWith override=0 must not invoke the runner")
	}
}

func TestFileWithUnknownPlatformFailsWithoutInvokingRunner(t *testing.T) {
	called := false
	err := fileWith("plan9", "", "/tmp/x.md", func(string, ...string) error {
		called = true
		return nil
	})
	if err == nil {
		t.Fatal("fileWith on an unknown platform with no override must fail")
	}
	if called {
		t.Fatal("fileWith on an unknown platform must not invoke the runner")
	}
}

func TestFileWithInvokesExactArgv(t *testing.T) {
	var gotName string
	var gotArgs []string
	err := fileWith("darwin", "", "/tmp/report.md", func(name string, args ...string) error {
		gotName, gotArgs = name, args
		return nil
	})
	if err != nil {
		t.Fatalf("fileWith: %v", err)
	}
	if gotName != "open" || !equalArgs(gotArgs, []string{"/tmp/report.md"}) {
		t.Fatalf("fileWith invoked %q %v, want open [/tmp/report.md]", gotName, gotArgs)
	}
}

func TestFileWithCustomOverrideInvokesExactArgv(t *testing.T) {
	var gotName string
	var gotArgs []string
	err := fileWith("linux", "wslview", "/tmp/report.md", func(name string, args ...string) error {
		gotName, gotArgs = name, args
		return nil
	})
	if err != nil {
		t.Fatalf("fileWith: %v", err)
	}
	if gotName != "wslview" || !equalArgs(gotArgs, []string{"/tmp/report.md"}) {
		t.Fatalf("fileWith invoked %q %v, want wslview [/tmp/report.md]", gotName, gotArgs)
	}
}

func TestFileWithPropagatesRunnerError(t *testing.T) {
	wantErr := fmt.Errorf("boom")
	err := fileWith("darwin", "", "/tmp/x.md", func(string, ...string) error {
		return wantErr
	})
	if !errors.Is(err, wantErr) {
		t.Fatalf("fileWith err = %v, want %v", err, wantErr)
	}
}
