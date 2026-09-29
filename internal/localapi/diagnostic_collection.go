package localapi

import (
	"context"
	"net/http"
)

type DiagnosticCollectionRequest struct {
	Action string `json:"action"`
	Peer   string `json:"peer"`
	ID     string `json:"id,omitempty"`
}

type DiagnosticCollectionResult struct {
	Code      string `json:"code"`
	ID        string `json:"id,omitempty"`
	State     string `json:"state,omitempty"`
	LocalPath string `json:"localPath,omitempty"`
	PeerPath  string `json:"peerPath,omitempty"`
}

func (c *Client) CollectDiagnostic(ctx context.Context, request DiagnosticCollectionRequest) (DiagnosticCollectionResult, error) {
	var result DiagnosticCollectionResult
	err := c.request(ctx, http.MethodPost, "/v1/diagnostic-collection", request, &result)
	return result, err
}

func (h *handler) diagnosticCollection(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var request DiagnosticCollectionRequest
	if decodeStrict(r.Body, &request) != nil || !validFingerprint(request.Peer) ||
		!(request.Action == "START" && request.ID == "" || request.Action == "STATUS" && validCollectionID(request.ID)) {
		writeJSON(w, http.StatusBadRequest, DiagnosticCollectionResult{Code: "invalid_request"})
		return
	}
	op, ok := h.operations.(interface {
		CollectDiagnostic(context.Context, DiagnosticCollectionRequest) (DiagnosticCollectionResult, error)
	})
	if !ok {
		writeJSON(w, http.StatusOK, DiagnosticCollectionResult{Code: "unsupported"})
		return
	}
	result, err := op.CollectDiagnostic(r.Context(), request)
	if err != nil {
		writeInternalFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func validCollectionID(id string) bool {
	if len(id) != 32 {
		return false
	}
	for _, c := range id {
		if c < '0' || c > '9' {
			if c < 'a' || c > 'f' {
				return false
			}
		}
	}
	return true
}
