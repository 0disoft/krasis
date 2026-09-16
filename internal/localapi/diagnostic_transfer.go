package localapi

import (
	"context"
	"net/http"
)

type DiagnosticTransferResult struct {
	Code string `json:"code"`
	Path string `json:"path,omitempty"`
}

func (c *Client) SendDiagnostic(ctx context.Context, peer string) (DiagnosticTransferResult, error) {
	var result DiagnosticTransferResult
	err := c.request(ctx, http.MethodPost, "/v1/diagnostic-transfer", struct {
		Peer string `json:"peer"`
	}{peer}, &result)
	return result, err
}
func (h *handler) diagnosticTransfer(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var request struct {
		Peer string `json:"peer"`
	}
	if decodeStrict(r.Body, &request) != nil || !validFingerprint(request.Peer) {
		writeJSON(w, 400, DiagnosticTransferResult{Code: "invalid_request"})
		return
	}
	op, ok := h.operations.(interface {
		SendDiagnostic(context.Context, string) (DiagnosticTransferResult, error)
	})
	if !ok {
		writeJSON(w, 200, DiagnosticTransferResult{Code: "unsupported"})
		return
	}
	result, err := op.SendDiagnostic(r.Context(), request.Peer)
	if err != nil {
		writeInternalFailure(w)
		return
	}
	writeJSON(w, 200, result)
}
