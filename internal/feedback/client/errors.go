package client

import (
	"errors"
	"fmt"
	"net/http"
)

// APIError reports a well-formed non-2xx HTTP response from the server: the
// status code, the server's own short generic message (errorResponseDTO's
// "error" field), and, for a 400, the field-level validation detail the
// server attaches. Message is always the server's own generic string (per
// httpapi's own design, it never echoes SQL/filesystem/token detail on a
// 5xx) — never something this client invents.
type APIError struct {
	StatusCode int
	Message    string
	Fields     []ValidationField
}

func (e *APIError) Error() string {
	if len(e.Fields) > 0 {
		return fmt.Sprintf("feedback client: server returned %d: %s (%d field error(s))", e.StatusCode, e.Message, len(e.Fields))
	}
	return fmt.Sprintf("feedback client: server returned %d: %s", e.StatusCode, e.Message)
}

// IsUnauthorized reports whether err is an *APIError for a 401 (missing or
// invalid bearer token / X-Identity-Id).
func IsUnauthorized(err error) bool { return statusIs(err, http.StatusUnauthorized) }

// IsForbidden reports whether err is an *APIError for a 403 (a
// recognized-but-wrong-scope credential, e.g. a consumer token presented to
// a user-scope endpoint).
func IsForbidden(err error) bool { return statusIs(err, http.StatusForbidden) }

// IsValidationError reports whether err is an *APIError for a 400
// (malformed JSON, or a field that failed validation — see its Fields).
func IsValidationError(err error) bool { return statusIs(err, http.StatusBadRequest) }

// IsTooLarge reports whether err is an *APIError for a 413 (request body
// exceeded the server's configured limit).
func IsTooLarge(err error) bool { return statusIs(err, http.StatusRequestEntityTooLarge) }

// IsRateLimited reports whether err is an *APIError for a 429 (the
// per-token write rate limit was exceeded).
func IsRateLimited(err error) bool { return statusIs(err, http.StatusTooManyRequests) }

// IsServiceUnavailable reports whether err is an *APIError for a 503 (the
// server is temporarily unable to serve the request — busy, or a
// maintenance job blocked it).
func IsServiceUnavailable(err error) bool { return statusIs(err, http.StatusServiceUnavailable) }

func statusIs(err error, code int) bool {
	var apiErr *APIError
	return errors.As(err, &apiErr) && apiErr.StatusCode == code
}

// NetworkError wraps a transport-level failure that never produced an HTTP
// response at all (connection refused, DNS failure, connection reset, ...).
// It is distinct from *TimeoutError so a caller can tell "the server is
// simply not reachable" apart from "the request didn't finish in time".
type NetworkError struct{ Err error }

func (e *NetworkError) Error() string {
	return fmt.Sprintf("feedback client: network error: %v", e.Err)
}
func (e *NetworkError) Unwrap() error { return e.Err }

// TimeoutError wraps a request that failed because it exceeded either the
// caller's context deadline or the client's own configured RequestTimeout.
type TimeoutError struct{ Err error }

func (e *TimeoutError) Error() string {
	return fmt.Sprintf("feedback client: request timed out: %v", e.Err)
}
func (e *TimeoutError) Unwrap() error { return e.Err }

// IsTimeout reports whether err is a *TimeoutError.
func IsTimeout(err error) bool {
	var t *TimeoutError
	return errors.As(err, &t)
}

// IsNetworkError reports whether err is a *NetworkError or a *TimeoutError
// (a timeout is also a network-level failure — no HTTP response was ever
// received — so it counts here too; use IsTimeout to distinguish the two).
func IsNetworkError(err error) bool {
	var n *NetworkError
	var t *TimeoutError
	return errors.As(err, &n) || errors.As(err, &t)
}

// CredentialError reports a problem reading or validating the local
// token_file/identity_file — missing, unreadable, empty, malformed, or too
// large. Path is included for debuggability; the file's *content* (the
// secret token or the identity) never is.
type CredentialError struct {
	Path string
	Op   string
	Err  error
}

func (e *CredentialError) Error() string {
	return fmt.Sprintf("feedback client: %s %s: %v", e.Op, e.Path, e.Err)
}
func (e *CredentialError) Unwrap() error { return e.Err }

// IsCredentialError reports whether err is a *CredentialError.
func IsCredentialError(err error) bool {
	var c *CredentialError
	return errors.As(err, &c)
}
