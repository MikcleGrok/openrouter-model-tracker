package httpapi

import (
	"context"
	"crypto/subtle"
	"net/http"
	"regexp"
	"strings"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
)

// identityPattern is this layer's canonical X-Identity-Id format (plan 6.1:
// "валидирует identity как канонический идентификатор фиксированного
// формата/длины"). internal/feedback deliberately leaves this undefined —
// IdentityID's own doc comment says the canonical wire format belongs to
// "whichever later layer authenticates a request", which is this package,
// not the domain service. The chosen format: exactly 64 lowercase hex
// characters (32 bytes / 256 bits) — long enough to be an unguessable
// client-generated pseudonym, fixed-length and easy to validate with a
// single regexp, and structurally incapable of colliding with any other
// header value.
var identityPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ValidIdentityFormat reports whether raw is a syntactically valid
// X-Identity-Id (see identityPattern's doc comment). Exported so
// cmd/feedback-server and any future client-side "feedback init" command
// can validate a generated identity against the exact same rule this
// package's auth middleware enforces, without duplicating the pattern.
func ValidIdentityFormat(raw string) bool {
	return identityPattern.MatchString(raw)
}

// authInfo is what a successful user-scope auth check attaches to the
// request context: the identity the caller authenticated as (from
// X-Identity-Id) and the raw bearer token presented (used only to key the
// per-token write rate limiter, plan 10.3 — never logged, never echoed back
// in any response).
type authInfo struct {
	identity feedback.IdentityID
	token    string
}

type ctxKey int

const authInfoCtxKey ctxKey = iota

func withAuthInfo(ctx context.Context, info authInfo) context.Context {
	return context.WithValue(ctx, authInfoCtxKey, info)
}

func authInfoFromContext(ctx context.Context) authInfo {
	info, _ := ctx.Value(authInfoCtxKey).(authInfo)
	return info
}

// identityFromContext returns the identity a requireUser-wrapped handler's
// request authenticated as. Calling it outside such a handler returns the
// zero IdentityID ("").
func identityFromContext(ctx context.Context) feedback.IdentityID {
	return authInfoFromContext(ctx).identity
}

// tokenFromContext returns the raw bearer token a requireUser-wrapped
// handler's request presented, for rate-limit keying only.
func tokenFromContext(ctx context.Context) string {
	return authInfoFromContext(ctx).token
}

// bearerToken extracts the token from a well-formed "Authorization: Bearer
// <token>" header. ok is false for a missing header, a different auth
// scheme, or an empty token.
func bearerToken(r *http.Request) (token string, ok bool) {
	h := r.Header.Get("Authorization")
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return "", false
	}
	token = strings.TrimSpace(strings.TrimPrefix(h, prefix))
	if token == "" {
		return "", false
	}
	return token, true
}

// tokenKind classifies a presented bearer token against this server's two
// known secrets (plan 6.3: route-binding is decided by which of the two
// configured static secrets was presented — never by anything the client
// claims).
type tokenKind int

const (
	tokenNone tokenKind = iota
	tokenUser
	tokenConsumer
)

// classifyToken compares presented against both configured secrets in
// constant time. An empty presented value, or an empty configured secret,
// never matches — constructing a Server with an empty token is a
// programmer/operator error (New rejects it for the user token; a nil
// consumer token simply means the consumer endpoint can never be reached),
// not something a request can accidentally satisfy.
func (s *Server) classifyToken(presented string) tokenKind {
	if presented == "" {
		return tokenNone
	}
	p := []byte(presented)
	if len(s.userToken) > 0 && subtle.ConstantTimeCompare(p, s.userToken) == 1 {
		return tokenUser
	}
	if len(s.consumerToken) > 0 && subtle.ConstantTimeCompare(p, s.consumerToken) == 1 {
		return tokenConsumer
	}
	return tokenNone
}

// authUser wraps next with the user-scope auth gate (plan 6.1): a valid
// Bearer token matching the configured user secret, plus a syntactically
// valid X-Identity-Id. On success, the identity and raw token are attached
// to the request context for the handler (and the rate limiter) to read.
//
// A token that matches the *consumer* secret instead of the user secret is
// a recognized-but-wrong-scope credential, not an unauthenticated request:
// it answers 403, matching plan 4.5's "User/TUI token на consumer endpoint
// получает 403" applied symmetrically in the other direction, and never a
// 401 that might invite a client to "just try harder" with a header it can
// never legitimately have here. Nothing about scope is ever read from a
// client-supplied header (X-Scope, X-Audience, or any other) — the only
// input to this decision is which of the two configured secrets, if
// either, byte-for-byte matches what was presented.
func (s *Server) authUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		switch s.classifyToken(token) {
		case tokenUser:
			// fall through to the identity check below
		case tokenConsumer:
			writeError(w, http.StatusForbidden, "token is not authorized for this endpoint")
			return
		default:
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}

		raw := r.Header.Get("X-Identity-Id")
		if !ValidIdentityFormat(raw) {
			writeError(w, http.StatusUnauthorized, "missing or invalid X-Identity-Id")
			return
		}

		ctx := withAuthInfo(r.Context(), authInfo{identity: feedback.IdentityID(raw), token: token})
		next(w, r.WithContext(ctx))
	}
}

// authConsumer wraps next with the trusted-consumer auth gate (plan
// 6.3/4.5): a valid Bearer token matching the configured consumer secret,
// bound at startup to the fixed audience "assistant-runtime" and the exact
// scope set {feedback:signal:read}. That binding is fixed server-side
// config, not something this function looks up per request — there is
// nothing else to check once the secret matches, since MVP credentials are
// static/file-bound rather than JWTs with parseable claims (plan 6.3: "MVP
// credential — static file-bound secret, не JWT: claims наружу не
// выдаются и не парсятся").
//
// This function never reads X-Scope, X-Audience, or any client-supplied
// claim — by construction, not by validation, since the code simply never
// looks at those headers, making it structurally impossible for a client
// to grant itself this scope by sending one. A user-scope token presented
// here answers 403 (a recognized-but-wrong-scope credential), matching
// plan 4.5 exactly; anything else (missing header, garbage token) answers
// 401.
func (s *Server) authConsumer(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
			return
		}
		switch s.classifyToken(token) {
		case tokenConsumer:
			next(w, r)
		case tokenUser:
			writeError(w, http.StatusForbidden, "token is not authorized for this endpoint")
		default:
			writeError(w, http.StatusUnauthorized, "missing or invalid bearer token")
		}
	}
}
