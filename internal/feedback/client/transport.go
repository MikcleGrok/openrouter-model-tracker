package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
)

// maxErrorBodyBytes bounds how much of a non-2xx response body this client
// will ever decode — defensive against a misbehaving server sending an
// oversized error body.
const maxErrorBodyBytes = 64 * 1024

// resolveURL builds the full request URL for pathSuffix against the
// client's base endpoint, with an optional raw (already-encoded) query
// string.
func (c *Client) resolveURL(pathSuffix, rawQuery string) *url.URL {
	u := *c.baseURL
	// Setting Path (not RawPath) directly is deliberate: pathSuffix may
	// contain a raw, unescaped model key with internal "/" characters (see
	// modelFeedbackPath), and url.URL.String() never percent-encodes "/"
	// that arrives via Path — only RawPath, left empty here, could cause
	// that. This is exactly what httpapi's routes.go expects: a model key's
	// internal slashes reaching the server unescaped, as its own tests send
	// them (e.g. "/v1/models/acme/model-1/feedback/me").
	u.Path = strings.TrimRight(u.Path, "/") + pathSuffix
	u.RawPath = ""
	u.RawQuery = rawQuery
	return &u
}

// modelFeedbackPath builds "/v1/models/<modelKey><suffix>". modelKey is
// inserted raw, never percent-escaped: a model key's allowed characters
// (letters, digits, "-_.:/" — internal/feedback.NormalizeModelKey's own
// policy) never need escaping in a URL path, and escaping its internal "/"
// would change what httpapi/routes.go's "{rest...}" wildcard receives.
func modelFeedbackPath(modelKey, suffix string) string {
	return "/v1/models/" + modelKey + suffix
}

// do sends an HTTP request: it applies the client's configured timeout on
// top of ctx, JSON-encodes body when non-nil, sets the auth headers
// (reading tokenFile/identityFile fresh on every call — never cached, so a
// rotated token takes effect without restarting this client), and maps a
// transport-level failure (no HTTP response at all) to
// *NetworkError/*TimeoutError. It does not interpret the response's status
// code — callers decode a 2xx body themselves or call responseError for
// anything else.
func (c *Client) do(ctx context.Context, method string, u *url.URL, body any) (*http.Response, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("feedback client: encode request body: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}

	req, err := http.NewRequestWithContext(ctx, method, u.String(), reader)
	if err != nil {
		return nil, fmt.Errorf("feedback client: build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	token, err := readToken(c.tokenFile)
	if err != nil {
		return nil, err
	}
	identity, err := readIdentity(c.identityFile)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Identity-Id", identity)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, mapTransportError(ctx, err)
	}
	return resp, nil
}

// mapTransportError classifies a failed http.Client.Do call: a timeout when
// the request's own context deadline expired or the underlying error
// itself reports Timeout(), a generic network error otherwise.
func mapTransportError(ctx context.Context, err error) error {
	if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return &TimeoutError{Err: err}
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return &TimeoutError{Err: err}
	}
	return &NetworkError{Err: err}
}

// responseError builds the typed error for a non-2xx response, consuming
// and closing resp.Body. It never returns nil.
func responseError(resp *http.Response) error {
	defer resp.Body.Close()
	var body errorResponseDTO
	// Best-effort decode: even an empty or non-JSON body (e.g. a proxy's
	// own error page) still leaves the status code alone enough to build a
	// meaningful *APIError.
	_ = json.NewDecoder(io.LimitReader(resp.Body, maxErrorBodyBytes)).Decode(&body)
	return &APIError{StatusCode: resp.StatusCode, Message: body.Error, Fields: body.Fields}
}

// decodeJSON decodes resp's body into v and closes it. Call only for a 2xx
// response.
func decodeJSON(resp *http.Response, v any) error {
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
		return fmt.Errorf("feedback client: decode response: %w", err)
	}
	return nil
}
