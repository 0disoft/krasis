package localapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/peerupdate"
	"io"
	"net/http"
)

const maxClipboardJSONBytes = ((peerupdate.MaxClipboardImageBytes + 2) / 3 * 4) + 4096

type ClipboardRequest struct {
	Action string  `json:"action"`
	Peer   string  `json:"peer"`
	Text   *string `json:"text,omitempty"`
	Format string  `json:"format,omitempty"`
	Data   []byte  `json:"data,omitempty"`
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
		return r.Text == nil && r.Format == "" && len(r.Data) == 0 && r.Ack == ""
	}
	if r.Action != "SYNC" || (r.Ack != "" && !validPairingID(r.Ack)) {
		return false
	}
	if r.Format == "" {
		return len(r.Data) == 0 && (r.Text == nil || len(*r.Text) <= peerupdate.MaxClipboardBytes)
	}
	return r.Text == nil && peerupdate.ValidClipboardPayload(r.Format, r.Data)
}
func (c *Client) Clipboard(ctx context.Context, r ClipboardRequest) (ClipboardResult, error) {
	var v ClipboardResult
	err := c.requestHeadersLimit(ctx, http.MethodPost, "/v1/clipboard", r, &v, nil, maxClipboardJSONBytes)
	return v, err
}
func (h *handler) clipboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	var req ClipboardRequest
	if decodeClipboardRequest(r.Body, &req) != nil || !req.Valid() {
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

func decodeClipboardRequest(r io.Reader, value *ClipboardRequest) error {
	// JSON may encode each text byte as six bytes (for example '<' as \u003c).
	const limit = maxClipboardJSONBytes
	data, err := io.ReadAll(io.LimitReader(r, limit+1))
	if err != nil {
		return err
	}
	if len(data) > limit {
		return errors.New("clipboard request too large")
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	if err = d.Decode(value); err != nil {
		return err
	}
	var extra any
	if d.Decode(&extra) != io.EOF {
		return errors.New("trailing clipboard request")
	}
	return nil
}
