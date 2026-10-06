package residentagent

import (
	"context"
	"errors"
	"os"
	"time"

	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peerupdate"
)

func (o *Operations) UIReady(ctx context.Context, pid uint32) (localapi.OperationResult, error) {
	if ctx.Err() != nil || !launchrecord.UIProcessMatches(o.localAppData, pid) {
		return rejected("ui_unverified"), nil
	}
	o.mu.Lock()
	defer o.mu.Unlock()
	o.uiReadyAt, o.uiReadyPID = time.Now(), pid
	return completed("ui_ready"), nil
}
func (o *Operations) uiReadySince(since time.Time) bool {
	o.mu.Lock()
	at, pid := o.uiReadyAt, o.uiReadyPID
	o.mu.Unlock()
	return at.After(since) && time.Since(at) < time.Minute && launchrecord.UIProcessMatches(o.localAppData, pid)
}

func (o *Operations) OpenApp(ctx context.Context, r localapi.AppOpenRequest) (localapi.AppOpenResult, error) {
	result := localapi.AppOpenResult{Code: "rejected"}
	if o.updates == nil || !o.updateAuthorized(ctx, "", r.Peer) {
		return result, nil
	}
	if r.Action == "STATUS" {
		result.ID, result.Code = r.ID, o.updates.AppOpenStatus(r.Peer, r.ID)
		return result, nil
	}
	if r.Action != "START" || r.ID != "" {
		return result, nil
	}
	id, err := o.updates.QueueAppOpen(r.Peer)
	if errors.Is(err, peerupdate.ErrBusy) {
		result.Code = "busy"
		return result, nil
	}
	if err == nil {
		result.Code, result.ID = "queued", id
	}
	return result, nil
}

func (o *Operations) openUIForPeer(peer string) string {
	p, ok := o.updatePeer(peer)
	if !ok || !p.AllowInboundControl || !o.updateAuthorized(o.root, p.Alias, peer) {
		return "rejected"
	}
	_, err := launchrecord.Relaunch(o.localAppData, "", nil)
	if err == nil {
		return "launch_requested"
	}
	if errors.Is(err, launchrecord.ErrMissing) || errors.Is(err, os.ErrNotExist) {
		return "launch_record_missing"
	}
	if errors.Is(err, launchrecord.ErrInvalid) {
		return "launch_record_invalid"
	}
	return "launch_failed"
}
