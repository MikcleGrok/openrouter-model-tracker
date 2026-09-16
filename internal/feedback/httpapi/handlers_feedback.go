package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/sboborikin/openrouter-model-tracker/internal/feedback"
	"github.com/sboborikin/openrouter-model-tracker/internal/feedback/sqlite"
)

// handlePutFeedback implements PUT /v1/models/{model_key}/feedback (plan
// 4.2). Called only from behind authUser (routes.go), so identityFromContext
// is always populated here.
func (s *Server) handlePutFeedback(w http.ResponseWriter, r *http.Request, rawModelKey string) {
	identity := identityFromContext(r.Context())

	if !s.putLimiter.Allow(tokenFromContext(r.Context())) {
		writeError(w, http.StatusTooManyRequests, "rate limit exceeded")
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, s.maxBodyBytes)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	var body feedbackRequestDTO
	if err := dec.Decode(&body); err != nil {
		writeDecodeError(w, err)
		return
	}
	if dec.More() {
		writeError(w, http.StatusBadRequest, "unexpected trailing data after JSON body")
		return
	}

	input, err := feedback.NewFeedbackInput(rawModelKey, body.Overall, toSkillRatings(body.Skills), body.Review)
	if err != nil {
		writeInputError(w, err)
		return
	}

	ctx := r.Context()
	own, err := s.service.SaveFeedback(ctx, identity, input)
	if err != nil {
		s.writeServiceError(w, err)
		return
	}

	resp, err := s.buildSummary(ctx, identity, input.ModelKey, &own, false)
	if err != nil {
		s.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleGetOwnFeedback implements GET /v1/models/{model_key}/feedback/me
// (plan 4.3). Absence of a rating is 200 with own_feedback: null, never 404.
func (s *Server) handleGetOwnFeedback(w http.ResponseWriter, r *http.Request, rawModelKey string) {
	modelKey, err := feedback.NormalizeModelKey(rawModelKey)
	if err != nil {
		writeInputError(w, err)
		return
	}
	identity := identityFromContext(r.Context())

	own, err := s.service.GetOwnFeedback(r.Context(), identity, modelKey)
	if err != nil {
		s.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ownFeedbackResponseDTO{
		ModelKey:    string(modelKey),
		OwnFeedback: newOwnFeedbackDTO(own),
	})
}

// handleGetSummary implements GET /v1/models/{model_key}/feedback/summary
// (plan 4.4). "others" is included only when the caller explicitly asks for
// it via ?others=true or ?others=1 — contract.md §5 leaves that decision to
// this layer; a query parameter (rather than, say, a header) keeps the
// request cacheable/bookmarkable and visible in access logs.
func (s *Server) handleGetSummary(w http.ResponseWriter, r *http.Request, rawModelKey string) {
	modelKey, err := feedback.NormalizeModelKey(rawModelKey)
	if err != nil {
		writeInputError(w, err)
		return
	}
	identity := identityFromContext(r.Context())
	includeOthers := wantOthers(r)

	resp, err := s.buildSummary(r.Context(), identity, modelKey, nil, includeOthers)
	if err != nil {
		s.writeServiceError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func wantOthers(r *http.Request) bool {
	v := r.URL.Query().Get("others")
	return v == "true" || v == "1"
}

// buildSummary computes the shared response shape for both PUT's 200 (plan
// 4.2) and GET .../summary's 200 (plan 4.4): mine, community, optionally
// others, and the three position signals. mine, if non-nil, is used
// directly (the PUT handler already has it from SaveFeedback and must not
// pay for a redundant read); if nil, it is fetched via GetOwnFeedback.
//
// base_position is always reported PositionStatusUnranked from this
// endpoint: Service.GetModelPositions' baseRanking is caller-supplied by
// design (ranking.go's own doc comment — "this package never computes that
// order itself"), and the feedback-server has no independent access to the
// benchmark/ranking order at all in this MVP (plan 3.1: the server does not
// depend on the TUI, and cmd/feedback-server's own flags, plan 7.2, name no
// source for one) — that order lives entirely client-side, in the TUI's
// local snapshot/model-map.tsv. Passing an empty ranking here is an honest
// input, not a workaround: personal_position and community_position are
// entirely unaffected by it (baseRanking is used only as a tie-breaker
// between equal scores, per ranking.go's lessByBaseRank), and reporting
// base_position as unranked is more accurate than fabricating one this
// service cannot actually know.
func (s *Server) buildSummary(ctx context.Context, identity feedback.IdentityID, modelKey feedback.ModelKey, mine *feedback.OwnFeedback, includeOthers bool) (summaryResponseDTO, error) {
	community, err := s.service.GetAggregate(ctx, modelKey, nil)
	if err != nil {
		return summaryResponseDTO{}, err
	}

	var othersDTO *aggregateDTO
	if includeOthers {
		exclude := identity
		others, err := s.service.GetAggregate(ctx, modelKey, &exclude)
		if err != nil {
			return summaryResponseDTO{}, err
		}
		othersDTO = newAggregateDTO(&others)
	}

	positions, err := s.service.GetModelPositions(ctx, identity, modelKey, nil)
	if err != nil {
		return summaryResponseDTO{}, err
	}

	if mine == nil {
		fetched, err := s.service.GetOwnFeedback(ctx, identity, modelKey)
		if err != nil {
			return summaryResponseDTO{}, err
		}
		mine = fetched
	}

	return summaryResponseDTO{
		ModelKey:          string(modelKey),
		Mine:              newOwnFeedbackDTO(mine),
		Community:         newAggregateDTO(&community),
		Others:            othersDTO,
		BasePosition:      newPositionDTO(positions.BasePosition),
		PersonalPosition:  newPositionDTO(positions.PersonalPosition),
		CommunityPosition: newPositionDTO(positions.CommunityPosition),
	}, nil
}

// writeDecodeError maps a JSON-decoding error to the right status: 413 for
// a body that exceeded the configured limit (http.MaxBytesReader/
// *http.MaxBytesError), 400 for anything else (syntax error, type
// mismatch, unknown field, EOF on an empty body).
func writeDecodeError(w http.ResponseWriter, err error) {
	var maxErr *http.MaxBytesError
	if errors.As(err, &maxErr) {
		writeError(w, http.StatusRequestEntityTooLarge, "request body too large")
		return
	}
	if errors.Is(err, io.EOF) {
		writeError(w, http.StatusBadRequest, "request body must not be empty")
		return
	}
	writeError(w, http.StatusBadRequest, "malformed JSON: "+err.Error())
}

// writeServiceError maps a Service/Store error to the right status,
// without ever echoing the underlying error's own text (which could be a
// SQL message or a filesystem path) — plan 4.2: "503/500 не должны
// раскрывать SQL, filesystem path или token". The underlying error is
// still logged server-side.
func (s *Server) writeServiceError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, feedback.ErrEmptyIdentity), errors.Is(err, feedback.ErrEmptyModelKey):
		// Both are internal-caller-contract guards (service.go's own doc
		// comments), never expected to actually trigger once auth
		// middleware and NormalizeModelKey have already run — treated as
		// 400 rather than 500 defensively, in case a future code path
		// reaches here without going through either.
		writeError(w, http.StatusBadRequest, "invalid request")
	case errors.Is(err, sqlite.ErrMaintenanceLocked), errors.Is(err, sqlite.ErrBusyTimeout):
		writeError(w, http.StatusServiceUnavailable, "service temporarily unavailable")
	default:
		s.logger.Error("internal error handling request", "error", err.Error())
		writeError(w, http.StatusInternalServerError, "internal error")
	}
}
