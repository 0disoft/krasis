package productpanel

import (
	"context"
	"encoding/hex"
	"errors"

	"github.com/0disoft/krasis/internal/localapi"
)

// A complete newer bundle explicitly opened by the user may interrupt the
// session. Unattended public release polling keeps its existing idle-only rule.
func (b *DesktopLifecycle) ApplyRequestedUpdate(generation int, revision string, launch func(string, string) error) (bool, error) {
	if launch == nil || !b.gate.TryLock() {
		return false, ErrStartupBusy
	}
	defer b.gate.Unlock()
	if b.updateHandedOff {
		return false, ErrAgentBusy
	}
	decoded, err := hex.DecodeString(revision)
	if err != nil || len(decoded) != 20 || generation <= localapi.AgentGeneration {
		return false, ErrAgentIncompatible
	}
	ctx, cancel := b.operation()
	defer cancel()
	bridge := NewDesktopBridge(ctx, b.open)
	view := bridge.Refresh()
	if view.Code != "ok" {
		return false, ErrAgentNotReady
	}
	switch view.State {
	case "LOCAL", "CONNECTED", "CONNECTING", "RECONNECTING":
	default:
		return false, ErrAgentBusy
	}
	client, err := b.open()
	if err != nil {
		return false, err
	}
	runtime, ok := client.(interface {
		Runtime(context.Context) (localapi.RuntimeInfo, error)
	})
	if !ok {
		return false, ErrAgentIncompatible
	}
	info, err := runtime.Runtime(ctx)
	if err != nil {
		return false, err
	}
	if info.Protocol != localapi.AgentProtocol || info.Generation != localapi.AgentGeneration {
		return false, ErrAgentIncompatible
	}
	alias, fingerprint := "", ""
	if view.State != "LOCAL" {
		alias = view.PeerAlias
		for _, peer := range view.Peers {
			if peer.Alias == alias {
				fingerprint = peer.Fingerprint
			}
		}
		if alias == "" || fingerprint == "" {
			return false, ErrAgentIncompatible
		}
		stopped := bridge.PauseForMaintenance()
		if stopped.Code != "ok" || stopped.State != "LOCAL" {
			b.save(stopped)
			return false, ErrAgentNotReady
		}
		b.recovery.Reset()
	}
	var launchErr error
	if b.applyReleaseUpdate(ctx, generation, revision, func() error {
		launchErr = launch(alias, fingerprint)
		return launchErr
	}) {
		return true, nil
	}
	// The existing claim rollback runs before restoring the previous session.
	// A user disconnect or shutdown cancels this operation and suppresses resume.
	if alias != "" && ctx.Err() == nil && b.ctx.Err() == nil {
		b.save(b.recoveryResume(bridge, alias, fingerprint))
	}
	if launchErr != nil {
		return false, launchErr
	}
	return false, errors.New("requested update handoff failed")
}
