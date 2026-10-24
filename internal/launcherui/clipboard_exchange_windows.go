//go:build windows

package launcherui

import (
	"context"

	"github.com/0disoft/krasis/internal/localapi"
)

// Only an explicit successful API result commits a prepared local copy. On
// failure re-read the current native clipboard next tick, rather than retaining
// old payload bytes that a newer user copy may already have superseded.
// Metadata-only failures keep their observation to avoid repacking rejected files.
// Applied remote copies retain only their receipt until a SYNC succeeds, without
// replaying their payload. A superseding local change must be sent without it.
func (p *clipboardPump) exchangeClipboard(ctx context.Context, previousSequence uintptr, request localapi.ClipboardRequest, send func(localapi.ClipboardRequest) (localapi.ClipboardResult, error)) (localapi.ClipboardResult, error) {
	if request.Action == "SYNC" && request.Ack == "" && request.Text == nil && request.Format == "" && !request.LocalChanged {
		request.Ack = p.pendingAck
	}
	var result localapi.ClipboardResult
	err := ctx.Err()
	if err == nil {
		result, err = send(request)
	}
	if err == nil && result.Code == "ok" {
		p.localChanged = false
		p.pendingAck = ""
	} else if ctx.Err() != nil || request.Text != nil || request.Format != "" {
		p.sequence = previousSequence
	}
	return result, err
}
