package client

import (
	"context"
	"net/http"
)

// PutFeedback submits (or, for an identity that already rated this model,
// updates — the server upserts on (identity, model_key)) this identity's
// feedback for modelKey, returning the resulting summary (mine, community,
// and the three position signals) exactly as the server computed it within
// the same request.
func (c *Client) PutFeedback(ctx context.Context, modelKey string, req FeedbackRequest) (Summary, error) {
	if req.Skills == nil {
		req.Skills = []SkillRating{}
	}
	u := c.resolveURL(modelFeedbackPath(modelKey, "/feedback"), "")
	resp, err := c.do(ctx, http.MethodPut, u, req)
	if err != nil {
		return Summary{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Summary{}, responseError(resp)
	}
	var out Summary
	if err := decodeJSON(resp, &out); err != nil {
		return Summary{}, err
	}
	return out, nil
}

// GetOwnFeedback reads this identity's own feedback for modelKey.
// OwnFeedbackResponse.OwnFeedback is nil when the identity has never rated
// this model — that is a normal 200 response, never an error.
func (c *Client) GetOwnFeedback(ctx context.Context, modelKey string) (OwnFeedbackResponse, error) {
	u := c.resolveURL(modelFeedbackPath(modelKey, "/feedback/me"), "")
	resp, err := c.do(ctx, http.MethodGet, u, nil)
	if err != nil {
		return OwnFeedbackResponse{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return OwnFeedbackResponse{}, responseError(resp)
	}
	var out OwnFeedbackResponse
	if err := decodeJSON(resp, &out); err != nil {
		return OwnFeedbackResponse{}, err
	}
	return out, nil
}

// GetSummary reads modelKey's summary: this identity's own rating, the
// community aggregate, and the three position signals. When includeOthers
// is true, the request asks the server (?others=true) to additionally
// populate Summary.Others with the community aggregate excluding this
// identity's own contribution.
func (c *Client) GetSummary(ctx context.Context, modelKey string, includeOthers bool) (Summary, error) {
	query := ""
	if includeOthers {
		query = "others=true"
	}
	u := c.resolveURL(modelFeedbackPath(modelKey, "/feedback/summary"), query)
	resp, err := c.do(ctx, http.MethodGet, u, nil)
	if err != nil {
		return Summary{}, err
	}
	if resp.StatusCode != http.StatusOK {
		return Summary{}, responseError(resp)
	}
	var out Summary
	if err := decodeJSON(resp, &out); err != nil {
		return Summary{}, err
	}
	return out, nil
}
