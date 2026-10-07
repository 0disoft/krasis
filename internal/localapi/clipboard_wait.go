package localapi

import (
	"context"
	"net/http"
	"time"
)

type ClipboardWaitRequest struct {
	Revision uint64 `json:"revision"`
}
type ClipboardWaitResult struct {
	Code     string `json:"code"`
	Revision uint64 `json:"revision"`
}

func (c *Client) WaitClipboard(ctx context.Context, revision uint64) (ClipboardWaitResult, error) {
	var value ClipboardWaitResult
	err := c.request(ctx, http.MethodPost, "/v1/clipboard/wait", ClipboardWaitRequest{Revision: revision}, &value)
	return value, err
}
func (h *handler) clipboardWait(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		writeMethodNotAllowed(w)
		return
	}
	var request ClipboardWaitRequest
	if decodeStrict(r.Body, &request) != nil {
		writeJSON(w, http.StatusBadRequest, ClipboardWaitResult{Code: "invalid_request"})
		return
	}
	op, ok := h.operations.(interface {
		WaitClipboard(context.Context, uint64) ClipboardWaitResult
	})
	if !ok {
		writeJSON(w, http.StatusOK, ClipboardWaitResult{Code: "unsupported"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, op.WaitClipboard(ctx, request.Revision))
}
