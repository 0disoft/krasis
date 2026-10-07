package productpanel

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/localapi"
)

func (s Startup) maintenanceCompatible(info localapi.RuntimeInfo, allowOlder bool) bool {
	return info.Protocol == localapi.AgentProtocol && (info.Generation == s.Generation && info.Build == s.Revision || allowOlder && info.Generation > 0 && info.Generation < s.Generation)
}

// Preflight happens before disconnect: never interrupt a session for an invalid
// bundle, an unknown/newer agent, pairing, or an initial optional installation.
func (s Startup) LaunchMaintenanceNeeded(ctx context.Context) (bool, error) {
	inputUpdate := false
	if s.InputProbe != nil {
		inputUpdate = s.InputProbe(ctx) == "UPDATE_REQUIRED"
	} else if s.InputSetup != nil {
		inputUpdate = s.InputSetup() == "UPDATE_REQUIRED"
	}
	if ctx.Err() != nil {
		return false, ctx.Err()
	}
	client, err := s.Open()
	if errors.Is(err, localapi.ErrUnavailable) || errors.Is(err, localapi.ErrTransport) {
		if !inputUpdate {
			return false, nil
		}
		return true, s.Verify()
	}
	if err != nil {
		return false, err
	}
	info, err := client.Runtime(ctx)
	if err != nil {
		return false, err
	}
	if !s.maintenanceCompatible(info, true) {
		return false, ErrAgentIncompatible
	}
	if !inputUpdate && info.Generation == s.Generation {
		return false, nil
	}
	if err := s.Verify(); err != nil {
		return false, err
	}
	status, err := client.Status(ctx)
	if err != nil {
		return false, err
	}
	switch status.State {
	case "LOCAL", "FAULTED", "CONNECTED", "CONNECTING", "RECONNECTING":
		return true, nil
	default:
		return false, ErrAgentBusy
	}
}

// Only Run's initial startup uses maintenance. Polling/recovery never repeats
// consent or disconnects a working session to refresh the installation.
func (b *DesktopLifecycle) initialize(ctx context.Context, initial BridgeSnapshot) BridgeSnapshot {
	if b.StartupMaintenanceNeeded != nil && b.StartupMaintenance != nil {
		needed, err := b.StartupMaintenanceNeeded(ctx)
		if err != nil {
			return b.failedStartup(ctx, err)
		}
		if needed {
			value := b.refreshProtectedInput(ctx, initial, b.StartupMaintenance)
			if b.pendingInput != nil {
				b.pendingInput.startup = true
			}
			prepared := value.Code == "ok" || value.agentPrepared
			if prepared && ctx.Err() == nil && b.StartupReady != nil {
				b.StartupReady()
			}
			if prepared && ctx.Err() == nil && b.StartupResume != nil {
				resumeValue := value
				// Login resume requires a current ordinary-agent observation. Only
				// replace an installer code, never a rejected/uncertain reconnect.
				if value.agentPrepared && value.Code == value.InputRefreshCode && value.ReconnectCode == "not_attempted" {
					resumeValue = NewDesktopBridge(ctx, b.open).Refresh()
				}
				resumed := b.StartupResume(ctx, resumeValue)
				resumed.InputRefreshCode, resumed.ReconnectCode = value.InputRefreshCode, value.ReconnectCode
				value = resumed
			}
			return value
		}
	}
	if err := b.startup(ctx); err != nil {
		return b.failedStartup(ctx, err)
	}
	value := NewDesktopBridge(ctx, b.open).Refresh()
	if b.StartupResume != nil {
		value = b.StartupResume(ctx, value)
	}
	return value
}
