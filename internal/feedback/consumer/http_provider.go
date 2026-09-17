package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// SupportedSchemaVersion is the only feedback-signal schema_version this
// reference adapter understands (httpapi/handlers_signal.go's own
// signalSchemaVersion constant, duplicated here per doc.go's decoupling
// rationale). A response carrying any other value is ErrIncompatibleSchema
// — never coerced or best-effort-decoded.
const SupportedSchemaVersion = "feedback-signal.v1"

// expectedSignalScope is the only signal_scope this adapter ever accepts
// (plan 4.6: the MVP consumer endpoint "всегда отдаёт только
// social/community signal... с signal_scope=community"). Any other value —
// including a hypothetical future "personal" — is treated as incompatible,
// never silently passed through (doc.go's "Community-only signal, by
// construction").
const expectedSignalScope = "community"

// DefaultRequestTimeout bounds a single GetSignal call when
// HTTPReferenceSignalProviderConfig.RequestTimeout is zero or negative.
const DefaultRequestTimeout = 3 * time.Second

// maxSignalBodyBytes bounds how much of the response body this adapter will
// ever read into memory — defensive against a misbehaving or compromised
// server sending an oversized body. A real signal response is at most a few
// KB even with every skill populated.
const maxSignalBodyBytes = 64 * 1024

// HTTPReferenceSignalProviderConfig configures a HTTPReferenceSignalProvider.
// BaseURL and Token are required.
type HTTPReferenceSignalProviderConfig struct {
	// BaseURL is the feedback-server's base URL, e.g.
	// "http://127.0.0.1:8787". Must be an absolute http/https URL with a
	// host.
	BaseURL string
	// Token is the trusted-consumer bearer credential (httpapi's separate
	// consumer secret, plan 6.3). This package never reads it from a file
	// itself and never logs it — how the caller stores, rotates, and
	// refreshes it is the external runtime's own concern, out of this
	// package's scope.
	Token string
	// HTTPClient overrides the *http.Client used to send requests. Defaults
	// to a plain &http.Client{} — RequestTimeout, applied per-request via
	// context, is what actually bounds a request.
	HTTPClient *http.Client
	// RequestTimeout bounds how long any single GetSignal call may run,
	// applied on top of whatever context.Context the caller passes
	// (whichever deadline is sooner wins). Defaults to
	// DefaultRequestTimeout.
	RequestTimeout time.Duration
}

// HTTPReferenceSignalProvider is the reference SignalProvider implementation
// that calls the real GET /v1/models/{model_key}/feedback/signal endpoint
// (httpapi/handlers_signal.go) with the consumer credential. Construct one
// with NewHTTPReferenceSignalProvider. Safe for concurrent use.
type HTTPReferenceSignalProvider struct {
	baseURL    *url.URL
	token      string
	httpClient *http.Client
	timeout    time.Duration
}

// NewHTTPReferenceSignalProvider validates cfg and constructs a
// HTTPReferenceSignalProvider.
func NewHTTPReferenceSignalProvider(cfg HTTPReferenceSignalProviderConfig) (*HTTPReferenceSignalProvider, error) {
	if cfg.BaseURL == "" {
		return nil, errors.New("consumer: base URL must not be empty")
	}
	u, err := url.Parse(cfg.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("consumer: invalid base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("consumer: base URL must use the http or https scheme, got %q", u.Scheme)
	}
	if u.Host == "" {
		return nil, errors.New("consumer: base URL must include a host")
	}
	if cfg.Token == "" {
		return nil, errors.New("consumer: token must not be empty")
	}

	timeout := cfg.RequestTimeout
	if timeout <= 0 {
		timeout = DefaultRequestTimeout
	}
	httpClient := cfg.HTTPClient
	if httpClient == nil {
		httpClient = &http.Client{}
	}

	base := *u
	return &HTTPReferenceSignalProvider{baseURL: &base, token: cfg.Token, httpClient: httpClient, timeout: timeout}, nil
}

// GetSignal implements SignalProvider by calling the real signal endpoint
// and mapping its response field-by-field into a FeedbackSignal. See
// contract.go's Err* doc comments for the exact error-mapping contract.
func (p *HTTPReferenceSignalProvider) GetSignal(ctx context.Context, modelKey string) (FeedbackSignal, error) {
	if strings.TrimSpace(modelKey) == "" {
		return FeedbackSignal{}, errors.New("consumer: model key must not be empty")
	}

	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p.resolveURL(modelKey).String(), nil)
	if err != nil {
		return FeedbackSignal{}, fmt.Errorf("consumer: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+p.token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return FeedbackSignal{}, &AdapterError{Err: ErrUnavailable, Detail: classifyTransportError(ctx, err)}
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(io.LimitReader(resp.Body, maxSignalBodyBytes))
		if err != nil {
			return FeedbackSignal{}, &AdapterError{Err: ErrUnavailable, Detail: "read response body"}
		}
		return decodeSignal(body)
	case http.StatusUnauthorized, http.StatusForbidden:
		return FeedbackSignal{}, &AdapterError{Err: ErrUnauthorized, Detail: fmt.Sprintf("http status %d", resp.StatusCode)}
	default:
		// Every other status — 5xx, and any 4xx this adapter does not
		// otherwise recognize (400 malformed model_key, 404, 429, ...) — is
		// treated as "could not obtain a usable signal", the same
		// ErrUnavailable a transport failure produces. None of these
		// statuses says anything about schema compatibility or policy, so
		// folding them into Unavailable is the honest classification: the
		// call simply did not produce a usable signal this time.
		return FeedbackSignal{}, &AdapterError{Err: ErrUnavailable, Detail: fmt.Sprintf("http status %d", resp.StatusCode)}
	}
}

// resolveURL builds "<base>/v1/models/<modelKey>/feedback/signal". modelKey
// is inserted raw, never percent-escaped, matching
// internal/feedback/client/transport.go's own modelFeedbackPath rationale:
// a model key's internal "/" characters must reach the server unescaped for
// httpapi/routes.go's "{rest...}" wildcard to recover it correctly.
func (p *HTTPReferenceSignalProvider) resolveURL(modelKey string) *url.URL {
	u := *p.baseURL
	u.Path = strings.TrimRight(u.Path, "/") + "/v1/models/" + modelKey + "/feedback/signal"
	u.RawPath = ""
	return &u
}

// classifyTransportError reports a short, safe, static description of a
// failed http.Client.Do call — "request timeout" when either the request's
// own context deadline expired or the underlying error itself reports
// Timeout(), "network error" otherwise. It never includes err's own message
// text (which could name a local file path or an internal address) in the
// returned string.
func classifyTransportError(ctx context.Context, err error) string {
	if ctx.Err() != nil && errors.Is(ctx.Err(), context.DeadlineExceeded) {
		return "request timeout"
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return "request timeout"
	}
	return "network error"
}

// --- Transport-only wire DTOs (mirror httpapi/dto.go's signal*DTO types) ---
//
// These are a distinct set of Go types, not an import of httpapi's
// (unexported) DTOs — see doc.go's decoupling rationale. Read
// httpapi/dto.go and httpapi/handlers_signal.go before changing anything
// here: they are the actual, reviewed, tested contract; this file must
// always match them, not the plan's illustrative example.

type consumerSignalDTO struct {
	ModelKey      string                          `json:"model_key"`
	SignalScope   string                          `json:"signal_scope"`
	Position      consumerPositionDTO             `json:"position"`
	Overall       consumerDimensionDTO            `json:"overall"`
	Skills        map[string]consumerDimensionDTO `json:"skills"`
	Status        string                          `json:"status"`
	Freshness     consumerFreshnessDTO            `json:"freshness"`
	SchemaVersion string                          `json:"schema_version"`
	PolicyVersion string                          `json:"policy_version"`
}

type consumerPositionDTO struct {
	CommunityPosition *int `json:"community_position"`
}

type consumerDimensionDTO struct {
	Value       *float64 `json:"value"`
	Status      string   `json:"status"`
	Confidence  *string  `json:"confidence"`
	SampleCount int      `json:"sample_count"`
}

type consumerFreshnessDTO struct {
	AsOf       *time.Time `json:"as_of"`
	ComputedAt time.Time  `json:"computed_at"`
	TTLSeconds int        `json:"ttl_seconds"`
	Stale      bool       `json:"stale"`
}

// requiredSignalKeys are the top-level JSON fields decodeSignal requires to
// be present (not merely absent-and-zero-valued) before it trusts the body
// as a real signal — the same set
// internal/feedback/httpapi/contract_test.go's
// TestContract_SignalResponse_ExactFieldNames checks the server actually
// emits.
var requiredSignalKeys = []string{
	"model_key", "signal_scope", "position", "overall", "skills",
	"status", "freshness", "schema_version", "policy_version",
}

// requiredDimensionKeys are the fields decodeSignal requires inside
// "overall" and every entry of "skills".
var requiredDimensionKeys = []string{"value", "status", "confidence", "sample_count"}

var validSignalStatuses = map[string]SignalStatus{
	"usable":              SignalUsable,
	"stale":               SignalStale,
	"unavailable":         SignalUnavailable,
	"incompatible_schema": SignalIncompatibleSchema,
	"policy_rejected":     SignalPolicyRejected,
}

var validDimensionStatuses = map[string]DimensionStatus{
	"insufficient":        DimensionInsufficient,
	"provisional":         DimensionProvisional,
	"established":         DimensionEstablished,
	"stale":               DimensionStale,
	"unavailable":         DimensionUnavailable,
	"incompatible_schema": DimensionIncompatibleSchema,
	"policy_rejected":     DimensionPolicyRejected,
}

var validConfidences = map[string]Confidence{
	"insufficient": ConfidenceInsufficient,
	"provisional":  ConfidenceProvisional,
	"established":  ConfidenceEstablished,
}

// decodeSignal validates and maps a 200 response body into a FeedbackSignal,
// or returns an *AdapterError wrapping ErrIncompatibleSchema/
// ErrPolicyRejected. It never returns a partially-populated FeedbackSignal
// alongside an error.
func decodeSignal(body []byte) (FeedbackSignal, error) {
	var raw map[string]any
	if err := json.Unmarshal(body, &raw); err != nil {
		return FeedbackSignal{}, &AdapterError{Err: ErrIncompatibleSchema, Detail: "malformed JSON body"}
	}
	if err := validateRawShape(raw); err != nil {
		return FeedbackSignal{}, &AdapterError{Err: ErrIncompatibleSchema, Detail: err.Error()}
	}

	var dto consumerSignalDTO
	if err := json.Unmarshal(body, &dto); err != nil {
		return FeedbackSignal{}, &AdapterError{Err: ErrIncompatibleSchema, Detail: "body does not match the expected signal shape"}
	}

	if dto.SchemaVersion != SupportedSchemaVersion {
		return FeedbackSignal{}, &AdapterError{Err: ErrIncompatibleSchema, Detail: "unsupported schema_version"}
	}
	if dto.SignalScope != expectedSignalScope {
		return FeedbackSignal{}, &AdapterError{Err: ErrIncompatibleSchema, Detail: "unsupported signal_scope"}
	}
	status, ok := validSignalStatuses[dto.Status]
	if !ok {
		return FeedbackSignal{}, &AdapterError{Err: ErrIncompatibleSchema, Detail: "unrecognized top-level status"}
	}

	overall, ok := mapDimension(dto.Overall)
	if !ok {
		return FeedbackSignal{}, &AdapterError{Err: ErrIncompatibleSchema, Detail: "unrecognized overall dimension shape"}
	}
	skills := make(map[string]Dimension, len(dto.Skills))
	for key, d := range dto.Skills {
		dim, ok := mapDimension(d)
		if !ok {
			return FeedbackSignal{}, &AdapterError{Err: ErrIncompatibleSchema, Detail: "unrecognized skill dimension shape"}
		}
		skills[key] = dim
	}

	// These two top-level statuses are well-formed, understood responses
	// that this adapter still refuses to hand back as usable data — they
	// become the corresponding typed error instead, exactly like a
	// transport-level failure (plan 3.4). Neither ever occurs from today's
	// real server (handlers_signal.go only ever sets usable/stale/
	// unavailable), but the wire contract documents them as valid future
	// values, so this adapter handles them rather than silently mismapping
	// them to "usable".
	switch status {
	case SignalPolicyRejected:
		return FeedbackSignal{}, &AdapterError{Err: ErrPolicyRejected, Detail: "server reported policy_rejected"}
	case SignalIncompatibleSchema:
		return FeedbackSignal{}, &AdapterError{Err: ErrIncompatibleSchema, Detail: "server reported incompatible_schema"}
	}

	return FeedbackSignal{
		ModelKey:          dto.ModelKey,
		SignalScope:       dto.SignalScope,
		CommunityPosition: dto.Position.CommunityPosition,
		Overall:           overall,
		Skills:            skills,
		Status:            status,
		Freshness: Freshness{
			AsOf:       dto.Freshness.AsOf,
			ComputedAt: dto.Freshness.ComputedAt,
			TTLSeconds: dto.Freshness.TTLSeconds,
			Stale:      dto.Freshness.Stale,
		},
		SchemaVersion: dto.SchemaVersion,
		PolicyVersion: dto.PolicyVersion,
	}, nil
}

// mapDimension converts one decoded consumerDimensionDTO into a Dimension,
// rejecting (ok=false) an unrecognized status or confidence value rather
// than passing an unknown string through.
func mapDimension(d consumerDimensionDTO) (dim Dimension, ok bool) {
	status, ok := validDimensionStatuses[d.Status]
	if !ok {
		return Dimension{}, false
	}
	var conf *Confidence
	if d.Confidence != nil {
		c, ok := validConfidences[*d.Confidence]
		if !ok {
			return Dimension{}, false
		}
		conf = &c
	}
	return Dimension{Value: d.Value, Status: status, Confidence: conf, SampleCount: d.SampleCount}, true
}

// validateRawShape checks raw for every required top-level key, and for
// "overall" and each entry of "skills" being an object carrying every
// required dimension key — the same field-presence rigor
// httpapi/contract_test.go applies when testing what the server emits,
// applied here to what this adapter is willing to trust as input.
func validateRawShape(raw map[string]any) error {
	for _, key := range requiredSignalKeys {
		if _, ok := raw[key]; !ok {
			return fmt.Errorf("missing required field %q", key)
		}
	}
	if _, ok := raw["freshness"].(map[string]any); !ok {
		return errors.New("\"freshness\" is not an object")
	}
	if _, ok := raw["position"].(map[string]any); !ok {
		return errors.New("\"position\" is not an object")
	}
	overall, ok := raw["overall"].(map[string]any)
	if !ok {
		return errors.New("\"overall\" is not an object")
	}
	if err := validateDimensionKeys(overall, "overall"); err != nil {
		return err
	}
	skills, ok := raw["skills"].(map[string]any)
	if !ok {
		return errors.New("\"skills\" is not an object")
	}
	for key, v := range skills {
		obj, ok := v.(map[string]any)
		if !ok {
			return fmt.Errorf("skills[%q] is not an object", key)
		}
		if err := validateDimensionKeys(obj, fmt.Sprintf("skills[%q]", key)); err != nil {
			return err
		}
	}
	return nil
}

func validateDimensionKeys(obj map[string]any, label string) error {
	for _, key := range requiredDimensionKeys {
		if _, ok := obj[key]; !ok {
			return fmt.Errorf("%s missing required field %q", label, key)
		}
	}
	return nil
}
