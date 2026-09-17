package client

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// identityPattern mirrors internal/feedback/httpapi/auth.go's own canonical
// X-Identity-Id format exactly (its identityPattern doc comment): 64
// lowercase hex characters, i.e. 32 random bytes. It is duplicated here
// rather than imported — see doc.go's "Deliberately decoupled" section for
// why — so any future change to httpapi's canonical definition must be
// mirrored here by hand.
var identityPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// identityByteLength is 32 bytes (256 bits), hex-encoding to the 64
// characters identityPattern requires.
const identityByteLength = 32

// maxCredentialFileBytes bounds how much of a token_file/identity_file this
// client will ever read into memory (plan 7.1's "ограниченный размер
// token", applied here to the actual secret content rather than to the
// config path string — internal/config's own FeedbackConfig bounds the path
// strings themselves). Both files are expected to hold one short value;
// this only exists to reject a misconfigured --token-file/--identity-file
// pointed at some much larger, unrelated file.
const maxCredentialFileBytes = 4096

// ValidIdentityFormat reports whether s is a syntactically valid
// X-Identity-Id, using the exact same rule httpapi's auth middleware
// enforces server-side.
func ValidIdentityFormat(s string) bool { return identityPattern.MatchString(s) }

// generateIdentityID returns a fresh cryptographically random identity,
// hex-encoded to identityPattern's canonical 64-lowercase-hex-character
// shape.
func generateIdentityID() (string, error) {
	buf := make([]byte, identityByteLength)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("feedback client: generate identity: %w", err)
	}
	return hex.EncodeToString(buf), nil
}

// readCredentialFile reads path's content, trimmed of surrounding
// whitespace, bounded by maxCredentialFileBytes.
func readCredentialFile(path string) (string, error) {
	if path == "" {
		return "", errors.New("path is empty")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, maxCredentialFileBytes+1))
	if err != nil {
		return "", err
	}
	if len(data) > maxCredentialFileBytes {
		return "", fmt.Errorf("file exceeds %d bytes", maxCredentialFileBytes)
	}
	return strings.TrimSpace(string(data)), nil
}

// readIdentity reads and validates identityFile's content against
// ValidIdentityFormat.
func readIdentity(path string) (string, error) {
	raw, err := readCredentialFile(path)
	if err != nil {
		return "", &CredentialError{Path: path, Op: "read identity file", Err: err}
	}
	if !ValidIdentityFormat(raw) {
		return "", &CredentialError{Path: path, Op: "read identity file", Err: errors.New("file does not contain a valid identity")}
	}
	return raw, nil
}

// readToken reads tokenFile's content: the shared bearer secret. Unlike an
// identity, a token's shape is opaque to this client — the server alone
// defines what its secret looks like — so this only bounds its size and
// rejects empty content; it never validates a format.
func readToken(path string) (string, error) {
	raw, err := readCredentialFile(path)
	if err != nil {
		return "", &CredentialError{Path: path, Op: "read token file", Err: err}
	}
	if raw == "" {
		return "", &CredentialError{Path: path, Op: "read token file", Err: errors.New("file is empty")}
	}
	return raw, nil
}

// writeSecretFile atomically creates or replaces path's content with data,
// mode 0600 in a 0700 parent directory (plan 6.1: "На Unix token-file и
// state-файл имеют mode 0600, каталог 0700") — the same
// create-temp-then-rename pattern cmd/feedback-server/tokenfile.go's own
// writeSecretFileAtomic uses, so a reader never observes a partially
// written identity file and a crash mid-write leaves any prior file intact.
//
// Full Windows ACL hardening (plan 6.1's "на Windows используется user-only
// ACL") is out of scope here: os.Chmod's permission bits are largely
// ignored by the Windows filesystem, and implementing the ACL equivalent
// needs platform-specific code this MVP client does not have elsewhere
// either.
func writeSecretFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".feedback-identity-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath) // no-op once the rename below succeeds

	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpPath, path)
}

// EnsureIdentityFile is the identity half of the idempotent client-side
// `feedback init` operation (plan 6.1: client-side init "только создаёт
// identity... не генерирует и не копирует secret"). If identityFile already
// holds a syntactically valid identity, it is left untouched and Created is
// false; otherwise (missing, unreadable, or malformed content — this does
// not distinguish "never existed" from "existed but was garbage", matching
// feedback-server's own token-init idempotency) a fresh cryptographically
// random identity is generated and written atomically.
func EnsureIdentityFile(identityFile string) (id string, created bool, err error) {
	if identityFile == "" {
		return "", false, errors.New("feedback client: identity file path is empty")
	}
	if existing, readErr := readIdentity(identityFile); readErr == nil {
		return existing, false, nil
	}
	newID, err := generateIdentityID()
	if err != nil {
		return "", false, err
	}
	if err := writeSecretFile(identityFile, []byte(newID+"\n")); err != nil {
		return "", false, fmt.Errorf("feedback client: write identity file %s: %w", identityFile, err)
	}
	return newID, true, nil
}

// CheckTokenFileReadable verifies tokenFile exists, is readable, and holds
// non-empty content within the bound this package enforces (plan 6.1:
// client-side init "проверяет доступность... token_file"). It never
// creates, generates, or copies the token itself — that is
// `feedback-server token init`'s job, on the server side.
func CheckTokenFileReadable(tokenFile string) error {
	if tokenFile == "" {
		return errors.New("feedback client: token file path is empty")
	}
	_, err := readToken(tokenFile)
	return err
}

// InitResult reports what Init (below) did.
type InitResult struct {
	// IdentityID is the identity now stored in identityFile — freshly
	// generated, or the one already there.
	IdentityID string
	// IdentityCreated is true only when Init generated a new identity;
	// false when identityFile already held a valid one.
	IdentityCreated bool
}

// Init is the idempotent client-side `feedback init` operation (plan 6.1).
// It creates identityFile if it does not yet hold a valid identity (or
// reuses the existing one), and verifies tokenFile is readable — but it
// never generates or copies a token/secret itself; provisioning the shared
// secret is `feedback-server token init`'s job on the server side. Calling
// Init again with an already-valid identityFile and a readable tokenFile is
// a no-op that simply reports the existing state.
func Init(identityFile, tokenFile string) (InitResult, error) {
	id, created, err := EnsureIdentityFile(identityFile)
	if err != nil {
		return InitResult{}, err
	}
	if err := CheckTokenFileReadable(tokenFile); err != nil {
		return InitResult{}, err
	}
	return InitResult{IdentityID: id, IdentityCreated: created}, nil
}
