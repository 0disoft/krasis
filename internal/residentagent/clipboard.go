package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
)

func (o *Operations) Clipboard(ctx context.Context, r localapi.ClipboardRequest) (localapi.ClipboardResult, error) {
	v := localapi.ClipboardResult{Code: "rejected"}
	if !r.Valid() || o.updates == nil {
		return v, nil
	}
	if r.Action == "DISABLE" {
		if o.updates.SetClipboardPeer("") == nil {
			v.Code = "ok"
		}
		return v, nil
	}
	if !o.updateAuthorized(ctx, "", r.Peer) {
		return v, nil
	}
	if r.Action == "ENABLE" {
		if o.updates.SetClipboardPeer(r.Peer) == nil {
			v.Code = "ok"
		}
		return v, nil
	}
	value, err := o.updates.SyncClipboard(r.Peer, r.Text, r.Ack)
	if err == nil {
		v.Code = "ok"
		v.Value = value
	}
	return v, nil
}
