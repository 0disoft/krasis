package localapi

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/peerupdate"
	"io"
	"net/http"
	"time"
)

const maxClipboardJSONBytes = ((peerupdate.MaxClipboardFileBytes + 2) / 3 * 4) + 4096
const clipboardRequestTimeout = 60 * time.Second

type ClipboardRequest struct {
	LocalChanged bool    `json:"localChanged,omitempty"`
	Report       string  `json:"report,omitempty"`
	Action       string  `json:"action"`
	Peer         string  `json:"peer"`
	Text         *string `json:"text,omitempty"`
	Format       string  `json:"format,omitempty"`
	Data         []byte  `json:"data,omitempty"`
	Ack          string  `json:"ack,omitempty"`
}
type ClipboardResult struct {
	Code  string                    `json:"code"`
	Value peerupdate.ClipboardValue `json:"value"`
}

func (r ClipboardRequest) Valid() bool {
	if r.LocalChanged && (r.Action != "SYNC" || r.Ack != "") {
		return false
	}
	if !validFingerprint(r.Peer) {
		return false
	}
	if r.Action == "REPORT" {
		if r.Text != nil || len(r.Data) != 0 || r.Format != "files" {
			return false
		}
		if r.Report == "apply_failed" {
			return validPairingID(r.Ack)
		}
		return r.Ack == "" && (r.Report == "preparing" || r.Report == "prepare_failed" || r.Report == "size_limit" || r.Report == "item_limit" || r.Report == "clipboard_changed")
	}
	if r.Report != "" {
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
	if c == nil || c.http == nil {
		return v, ErrUnavailable
	}
	client := *c
	client.http = new(http.Client)
	*client.http = *c.http
	client.http.Timeout = clipboardRequestTimeout
	err := client.requestHeadersLimit(ctx, http.MethodPost, "/v1/clipboard", r, &v, nil, maxClipboardJSONBytes)
	return v, err
}
func (h *handler) clipboard(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeMethodNotAllowed(w)
		return
	}
	deadline := time.Now().Add(clipboardRequestTimeout)
	controller := http.NewResponseController(w)
	_ = controller.SetReadDeadline(deadline)
	_ = controller.SetWriteDeadline(deadline)
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
	// Decode directly from the bounded stream instead of retaining a second
	// full JSON buffer in addition to the decoder and decoded base64 payload.
	limited := &io.LimitedReader{R: r, N: maxClipboardJSONBytes + 1}
	d := json.NewDecoder(limited)
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return errors.Join(errors.New("trailing clipboard request"), err)
	}
	// EOF from LimitedReader alone does not prove the body fit. The extra
	// sentinel byte also counts when it is only trailing JSON whitespace.
	if limited.N == 0 {
		return errors.New("clipboard request too large")
	}
	return nil
}
