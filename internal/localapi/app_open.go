package localapi

import (
	"context"
	"net/http"
)

type AppOpenRequest struct {
	Action string `json:"action"`
	Peer   string `json:"peer"`
	ID     string `json:"id,omitempty"`
}

type AppOpenResult struct {
	Code string `json:"code"`
	ID   string `json:"id,omitempty"`
}

func (c *Client) OpenApp(ctx context.Context, request AppOpenRequest) (AppOpenResult, error) {
	var result AppOpenResult
	err := c.request(ctx, http.MethodPost, "/v1/app-open", request, &result)
	return result, err
}

func (h *handler) appOpen(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var request AppOpenRequest
	if decodeStrict(r.Body, &request) != nil || !validFingerprint(request.Peer) ||
		!(request.Action == "START" && request.ID == "" || request.Action == "STATUS" && validCollectionID(request.ID)) {
		writeJSON(w, http.StatusBadRequest, AppOpenResult{Code: "invalid_request"})
		return
	}
	op, ok := h.operations.(interface {
		OpenApp(context.Context, AppOpenRequest) (AppOpenResult, error)
	})
	if !ok {
		writeJSON(w, http.StatusOK, AppOpenResult{Code: "unsupported"})
		return
	}
	result, err := op.OpenApp(r.Context(), request)
	if err != nil {
		writeInternalFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
