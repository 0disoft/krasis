package productcli

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productlog"
	"time"
)

// A scheduled recovery never interrupts a responding or still-owned agent.
// Startup preferences, explicit stop, image integrity and the instance lease
// are rechecked locally; it opens no remote command channel.
func ensureApp(ctx context.Context, deps dependencies) (result relaunchResult, exit int) {
	result = relaunchResult{SchemaVersion: "krasis.cli.relaunch/v1"}
	defer func() {
		saved := productlog.SaveRecovery(deps.localAppData, productlog.RecoverySummary{Time: time.Now().UTC().Format(time.RFC3339Nano), Code: result.Code, Generation: result.Generation, AgentState: result.AgentState}) == nil
		result.DiagnosticSaved = &saved
	}()
	settings, err := appsettings.Load(deps.localAppData)
	if err != nil {
		result.Code = "settings_unavailable"
		return result, ExitUnsafeState
	}
	paused, err := appsettings.RecoveryPaused(deps.localAppData)
	if err != nil {
		result.Code = "settings_unavailable"
		return result, ExitUnsafeState
	}
	if !settings.StartAtLogin || paused {
		result.Code = "recovery_disabled"
		return result, ExitOK
	}
	control, err := ResolveControlDirectory(deps.localAppData, "")
	if err != nil {
		result.Code = "agent_unavailable"
		return result, ExitUnsafeState
	}
	probe, done := context.WithTimeout(ctx, 2*time.Second)
	c, err := deps.openClient(control)
	var status localapi.Status
	if err == nil {
		status, err = c.Status(probe)
	}
	done()
	if err == nil {
		result.AgentState = status.State
		switch status.State {
		case "FAULTED":
			result.Code = "agent_faulted"
			return result, ExitUnsafeState
		case "LOCAL", "CONNECTED", "CONNECTING", "RECONNECTING", "DISCONNECTING":
		default:
			result.AgentState, result.Code = "UNKNOWN", "agent_unverified"
			return result, ExitUnsafeState
		}
		result.Code = "agent_present"
		return result, ExitOK
	}
	if !errors.Is(err, localapi.ErrUnavailable) && !errors.Is(err, localapi.ErrTransport) {
		result.Code = "agent_unverified"
		return result, ExitUnsafeState
	}
	available := deps.agentAvailable
	if available == nil {
		available = func() (bool, error) {
			lease, err := agentinstance.Acquire()
			if errors.Is(err, agentinstance.ErrAlreadyRunning) {
				return false, nil
			}
			if err != nil {
				return false, err
			}
			return true, lease.Close()
		}
	}
	free, err := available()
	if err != nil {
		result.Code = "agent_unverified"
		return result, ExitUnsafeState
	}
	if !free {
		result.Code = "agent_present"
		result.AgentState = "UNKNOWN"
		return result, ExitOK
	}
	if _, err = launchrecord.Inspect(deps.localAppData); err != nil {
		result.Code = "launch_record_invalid"
		return result, ExitUnsafeState
	}
	if deps.launch == nil {
		deps.launch = launchrecord.StartAtLogin
	}
	return relaunch(deps)
}
