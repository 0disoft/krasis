package localapi

import (
	"context"
	"github.com/0disoft/krasis/internal/peerupdate"
	"net/http"
)

type ClipboardRequest struct {
	Action string  `json:"action"`
	Peer   string  `json:"peer"`
	Text   *string `json:"text,omitempty"`
	Ack    string  `json:"ack,omitempty"`
}
type ClipboardResult struct {
	Code  string                    `json:"code"`
	Value peerupdate.ClipboardValue `json:"value"`
}

func (r ClipboardRequest) Valid() bool {
	if !validFingerprint(r.Peer) {
		return false
	}
	if r.Action == "ENABLE" || r.Action == "DISABLE" {
		return r.Text == nil && r.Ack == ""
	}
	return r.Action == "SYNC" && (r.Ack == "" || validPairingID(r.Ack)) && (r.Text == nil || len(*r.Text) <= peerupdate.MaxClipboardBytes)
}
func (c *Client) Clipboard(ctx context.Context, r ClipboardRequest) (ClipboardResult, error) {
	var v ClipboardResult
	err := c.request(ctx, http.MethodPost, "/v1/clipboard", r, &v)
	return v, err
}
func (h *handler) clipboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req ClipboardRequest
	if decodeStrict(r.Body, &req) != nil || !req.Valid() {
		writeJSON(w, 400, ClipboardResult{Code: "invalid_request"})
		return
	}
	op, ok := h.operations.(interface {
		Clipboard(context.Context, ClipboardRequest) (ClipboardResult, error)
	})
	if !ok {
		writeJSON(w, 200, ClipboardResult{Code: "unsupported"})
		return
	}
	result, err := op.Clipboard(r.Context(), req)
	if err != nil {
		writeInternalFailure(w)
		return
	}
	writeJSON(w, 200, result)
}
