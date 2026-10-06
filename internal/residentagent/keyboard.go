package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/keyboardstate"
	"github.com/0disoft/krasis/internal/localapi"
)

func (o *Operations) Keyboard(ctx context.Context, request localapi.KeyboardRequest) (localapi.KeyboardResult, error) {
	v := localapi.KeyboardResult{Schema: localapi.KeyboardSchema, Code: "unavailable", Peer: request.Peer, Remote: keyboardstate.Unknown()}
	if !request.Valid() || ctx.Err() != nil || o.updates == nil {
		return v, nil
	}
	if !o.updates.ReportKeyboard(request.Local, o.now()) {
		return v, nil
	}
	if request.Peer != "" && !o.updateAuthorized(ctx, "", request.Peer) {
		v.Code = "rejected"
		return v, nil
	}
	if request.Peer != "" {
		v.Remote = o.updates.PeerKeyboard(request.Peer, o.now())
	}
	v.Code = "ok"
	return v, nil
}
