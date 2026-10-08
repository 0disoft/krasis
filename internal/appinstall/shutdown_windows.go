//go:build windows

package appinstall

import (
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
)

var ErrShutdown = errors.New("Krasis shutdown not confirmed")

type shutdownError struct {
	stage string
	cause error
}

func (e *shutdownError) Error() string {
	return fmt.Sprintf("%s: %s: %v", ErrShutdown, e.stage, e.cause)
}
func (e *shutdownError) Unwrap() []error { return []error{ErrShutdown, e.cause} }
func ShutdownStage(err error) string {
	var failure *shutdownError
	if errors.As(err, &failure) {
		return failure.stage
	}
	return ""
}

type setupAgent interface {
	Runtime(context.Context) (localapi.RuntimeInfo, error)
	Status(context.Context) (localapi.Status, error)
	PauseForMaintenance(context.Context) (localapi.OperationResult, error)
	StopForUpgrade(context.Context, string, int, string) (localapi.OperationResult, error)
}
type setupShutdownOps struct {
	open func() (setupAgent, error)
	idle func() (io.Closer, error)
	quit func(context.Context) error
	wait func(context.Context) error
}

func waitSetupStartup(ctx context.Context) (*agentinstance.Lease, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tick := time.NewTicker(25 * time.Millisecond)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-tick.C:
			lease, err := agentinstance.AcquireControlStartup()
			if !errors.Is(err, agentinstance.ErrAlreadyRunning) {
				return lease, err
			}
		}
	}
}

func stopForSetup(ctx context.Context, base string, generation int, revision string) (io.Closer, error) {
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	ui, err := launchrecord.PrepareUIShutdown(base)
	if err != nil {
		return nil, &shutdownError{"ui_ownership", err}
	}
	defer ui.Close()
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return nil, err
	}
	return shutdownSetup(ctx, generation, revision, setupShutdownOps{
		open: func() (setupAgent, error) { return localapi.OpenClient(paths.Control) },
		idle: func() (io.Closer, error) { return agentinstance.Acquire() },
		quit: ui.Stop,
		wait: func(ctx context.Context) error {
			timer := time.NewTimer(25 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	})
}

// All preflight reads precede UI shutdown. An idle-agent lease is retained until
// publication finishes, and the enclosing installer holds the startup lease.
func shutdownSetup(ctx context.Context, generation int, revision string, ops setupShutdownOps) (io.Closer, error) {
	fail := func(stage string, err error) (io.Closer, error) {
		return nil, &shutdownError{stage, err}
	}
	if err := ctx.Err(); err != nil {
		return fail("cancelled", err)
	}
	idle, err := ops.idle()
	if err == nil {
		if err := ops.quit(ctx); err != nil {
			idle.Close()
			return fail("UI exit", err)
		}
		return idle, nil
	}
	if !errors.Is(err, agentinstance.ErrAlreadyRunning) {
		return fail("agent ownership", err)
	}
	client, err := ops.open()
	if err != nil {
		return fail("agent API", err)
	}
	info, err := client.Runtime(ctx)
	if err != nil {
		return fail("agent identity", err)
	}
	if info.Protocol != localapi.AgentProtocol || info.Generation < 226 || info.Generation > generation ||
		(info.Generation == generation && info.Build != revision) {
		return fail("agent identity", ErrOlder)
	}
	status, err := client.Status(ctx)
	if err != nil {
		return fail("agent state", err)
	}
	switch status.State {
	case "LOCAL", "FAULTED", "CONNECTED", "CONNECTING", "RECONNECTING":
	default:
		return fail("agent busy", ErrBusy)
	}
	if err = ops.quit(ctx); err != nil {
		return fail("UI exit", err)
	}
	paused, err := client.PauseForMaintenance(ctx)
	if err != nil {
		return fail("disconnect", err)
	}
	if !(paused.Status == "COMPLETED" && paused.Code == "disconnected") &&
		!(paused.Status == "REJECTED" && paused.Code == "no_active_session") {
		return fail("disconnect", errors.New(paused.Code))
	}
	for {
		if err := ctx.Err(); err != nil {
			return fail("cancelled", err)
		}
		stopped, err := client.StopForUpgrade(ctx, info.Instance, generation, revision)
		if err != nil {
			return fail("agent exit request", err)
		}
		if stopped.Status == "COMPLETED" && stopped.Code == "upgrade_stop_requested" {
			break
		}
		if stopped.Status != "REJECTED" || stopped.Code != "guard_exit_pending" {
			return fail("agent exit request", errors.New(stopped.Code))
		}
		if err := ops.wait(ctx); err != nil {
			return fail("guard exit", err)
		}
	}
	for {
		if err := ctx.Err(); err != nil {
			return fail("agent exit", err)
		}
		idle, err := ops.idle()
		if err == nil {
			return idle, nil
		}
		if !errors.Is(err, agentinstance.ErrAlreadyRunning) {
			return fail("agent ownership", err)
		}
		if err := ops.wait(ctx); err != nil {
			return fail("agent exit", err)
		}
	}
}
