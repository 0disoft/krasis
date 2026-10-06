package localapi

import (
	"context"
	"github.com/0disoft/krasis/internal/keyboardstate"
	"net/http"
)

const KeyboardSchema = "krasis.keyboard-state/v1"

type KeyboardRequest struct {
	Schema string              `json:"schema"`
	Peer   string              `json:"peer,omitempty"`
	Local  keyboardstate.State `json:"local"`
}
type KeyboardResult struct {
	Schema string              `json:"schema"`
	Code   string              `json:"code"`
	Peer   string              `json:"peer,omitempty"`
	Remote keyboardstate.State `json:"remote"`
}

func (r KeyboardRequest) Valid() bool {
	return r.Schema == KeyboardSchema && (r.Peer == "" || validFingerprint(r.Peer)) && r.Local.Valid()
}
func (c *Client) Keyboard(ctx context.Context, request KeyboardRequest) (KeyboardResult, error) {
	var result KeyboardResult
	err := c.request(ctx, http.MethodPost, "/v1/keyboard", request, &result)
	if err == nil && (result.Schema != KeyboardSchema || result.Peer != request.Peer || !result.Remote.Valid()) {
		err = ErrProtocol
	}
	return result, err
}
func (h *handler) keyboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
		writeMethodNotAllowed(w)
		return
	}
	var request KeyboardRequest
	if decodeStrict(r.Body, &request) != nil || !request.Valid() {
		writeJSON(w, http.StatusBadRequest, KeyboardResult{Schema: KeyboardSchema, Code: "rejected", Remote: keyboardstate.Unknown()})
		return
	}
	op, ok := h.operations.(interface {
		Keyboard(context.Context, KeyboardRequest) (KeyboardResult, error)
	})
	if !ok {
		writeJSON(w, http.StatusOK, KeyboardResult{Schema: KeyboardSchema, Code: "unsupported", Peer: request.Peer, Remote: keyboardstate.Unknown()})
		return
	}
	result, err := op.Keyboard(r.Context(), request)
	if err != nil {
		writeInternalFailure(w)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
