package client

import (
	"context"
	"encoding/json"
	"net/http"
)

// DeleteResult reports the outcome of a successful DeleteMe call —
// "successful" meaning this identity's delete actually committed on the
// server, whether or not its background cleanup phase has fully finished
// yet.
type DeleteResult struct {
	// Pending is true for a 202 cleanup_pending response (this identity's
	// delete committed; the server's post-commit cleanup did not finish
	// within this one request) and false for a 204 (fully done). A caller
	// that needs the delete to be completely finished, not merely
	// committed, should call DeleteMe again (it is idempotent) until
	// Pending is false.
	Pending bool
}

// DeleteMe deletes this identity's data, distinguishing three outcomes
// (Task 4's own reviewed behavior for DELETE /v1/me/feedback, carried
// forward here):
//
//   - 204: (DeleteResult{Pending: false}, nil) — fully done.
//   - 202 cleanup_pending: (DeleteResult{Pending: true}, nil) — this
//     identity's own delete committed on the server in this request; treat
//     it as success, not failure. Call DeleteMe again later if the caller
//     needs to confirm cleanup has fully finished.
//   - Anything else (e.g. a 503 from a persistent server-side problem):
//     (DeleteResult{}, err) with a non-nil *APIError/*NetworkError/
//     *TimeoutError — this identity's delete did NOT commit in this
//     request. A caller should treat this as a failure it may retry, never
//     as "still pending".
func (c *Client) DeleteMe(ctx context.Context) (DeleteResult, error) {
	u := c.resolveURL("/v1/me/feedback", "")
	resp, err := c.do(ctx, http.MethodDelete, u, nil)
	if err != nil {
		return DeleteResult{}, err
	}
	switch resp.StatusCode {
	case http.StatusNoContent:
		resp.Body.Close()
		return DeleteResult{Pending: false}, nil
	case http.StatusAccepted:
		defer resp.Body.Close()
		var body deleteStatusDTO
		// Best-effort decode: the status code alone already establishes
		// "pending" per the documented contract; a malformed or empty body
		// here would still be an accurate DeleteResult.
		_ = json.NewDecoder(resp.Body).Decode(&body)
		return DeleteResult{Pending: true}, nil
	default:
		return DeleteResult{}, responseError(resp)
	}
}
