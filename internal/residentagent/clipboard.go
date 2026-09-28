package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

func (o *Operations) Clipboard(ctx context.Context, r localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
	v := localapi.ClipboardResult{Code: "rejected"}
	if !r.Valid() || o.updates == nil {
		return v, nil
	}
	if r.Action == "DISABLE" {
		if cleared, err := o.updates.ClearClipboardPeer(r.Peer); err == nil && cleared {
			v.Code = "ok"
		}
		return v, nil
	}
	if !o.updateAuthorized(ctx, "", r.Peer) {
		return v, nil
	}
	if r.Action == "ENABLE" {
		if enabled, err := o.updates.EnableClipboardPeer(r.Peer); err == nil && enabled {
			v.Code = "ok"
		}
		return v, nil
	}
	var local *peerupdate.ClipboardValue
	if r.Text != nil || r.Format != "" {
		local = &peerupdate.ClipboardValue{Format: r.Format, Data: r.Data}
		if r.Text != nil {
			local.Text = *r.Text
		}
	}
	value, err := o.updates.SyncClipboardPayload(r.Peer, local, r.Ack)
	if err == nil {
		v.Code = "ok"
		v.Value = value
	}
	return v, nil
}
