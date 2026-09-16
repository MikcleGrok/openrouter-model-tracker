package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
)

// secretBytes is how many random bytes back a generated secret (256 bits) —
// generous for a bearer token compared with a constant-time byte-for-byte
// comparison (auth.go's classifyToken), encoded as hex so the file stays
// plain, greppable text.
const secretBytes = 32

// generateSecretHex returns a fresh cryptographically random secret,
// hex-encoded.
func generateSecretHex() (string, error) {
	b := make([]byte, secretBytes)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("generate random secret: %w", err)
	}
	return hex.EncodeToString(b), nil
}

// readSecretFile reads a token file written by writeSecretFileExclusive/
// writeSecretFileAtomic, trimming surrounding whitespace (a single trailing
// newline is what this package always writes, but a hand-edited file with
// different whitespace should still work). It refuses an empty result — a
// blank/whitespace-only token file is a misconfiguration, not a valid
// "no auth" state (plan 6.1: the shared secret is always required).
func readSecretFile(path string) ([]byte, error) {
	if path == "" {
		return nil, fmt.Errorf("no path given")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, fmt.Errorf("%s is empty", path)
	}
	return trimmed, nil
}

// writeSecretFileExclusive creates path with mode 0600 (plan 6.1: "На Unix
// token-file... имеет mode 0600, каталог 0700") and writes secret plus a
// trailing newline. It refuses to overwrite an existing file — creating a
// new secret is a deliberate, rare operation, and silently clobbering a
// live token-file that a running server (or another client) already trusts
// would be exactly the kind of accidental footgun this guards against.
// The parent directory is created (mode 0700) if it does not exist.
func writeSecretFileExclusive(path, secret string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", filepath.Dir(path), err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("%s already exists; refusing to overwrite (rotate or remove it first)", path)
		}
		return fmt.Errorf("create %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.WriteString(secret + "\n"); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return f.Close()
}

// writeSecretFileAtomic atomically replaces path's content with secret
// (plan 6.1/6.3: "атомарно заменить token-file с теми же permissions"),
// via a temp-file-then-rename in the same directory so a reader never sees
// a partially-written file, and a crash mid-write leaves the old secret
// intact rather than a corrupt one. Unlike writeSecretFileExclusive, this
// succeeds whether or not path already existed — rotation's job is "make
// this path contain a fresh secret," not "there must already be one."
func writeSecretFileAtomic(path, secret string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create directory %s: %w", dir, err)
	}
	tmp, err := os.CreateTemp(dir, ".feedback-token-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return fmt.Errorf("chmod temporary file: %w", err)
	}
	if _, err := tmp.WriteString(secret + "\n"); err != nil {
		tmp.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
