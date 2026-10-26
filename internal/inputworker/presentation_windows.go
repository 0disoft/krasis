//go:build windows

package inputworker

import (
	"context"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/wininput"
)

// PresentReturnCursor uses the same installed image, SCM and pipe-owner proofs
// as input. The returned channel is a cancellation handle, never an InputPort.
func (service *AgentService) PresentReturnCursor(ctx context.Context, lease Lease) (func() error, error) {
	return service.PresentReturnCursorObserved(ctx, lease, nil)
}

func (service *AgentService) PresentReturnCursorObserved(ctx context.Context, lease Lease, emit func(string)) (func() error, error) {
	if service == nil || service.installation == nil || ctx == nil || ctx.Err() != nil || !lease.Presentation {
		return nil, ErrAuthority
	}
	data, err := encodeLease(lease, time.Now())
	if err != nil {
		return nil, err
	}
	started := time.Now()
	channel, err := connectInstalledService(ctx, service.digest, service.installation)
	observePresentationPhase(emit, "pipe_connect", time.Since(started))
	if err != nil {
		return nil, err
	}
	accepted := false
	defer func() {
		if !accepted {
			_ = channel.Close()
		}
	}()
	finish := startupCancellation(ctx, channel)
	defer finish()
	deadline := time.Now().Add(time.Second)
	if limit, ok := ctx.Deadline(); ok && limit.Before(deadline) {
		deadline = limit
	}
	if channel.stream.SetDeadline(deadline) != nil || writeFrame(channel.stream, data[:]) != nil {
		return nil, ErrAuthority
	}
	profile, err := readStartupReady(channel.stream)
	if err != nil || ctx.Err() != nil {
		return nil, ErrAuthority
	}
	observePresentationStartup(emit, profile)
	if channel.stream.SetDeadline(time.Time{}) != nil {
		return nil, ErrAuthority
	}
	if finish() != nil {
		return nil, ErrAuthority
	}
	accepted = true
	return channel.Close, nil
}

func runCursorPresentation(ctx context.Context, channel, service *Channel, lease Lease, ready func() error) (result error) {
	run, cancel := context.WithDeadline(ctx, lease.ExpiresAt)
	defer cancel()
	done := make(chan struct{})
	// Presentation accepts EOF/cancellation only. Any request bytes retire it;
	// there is no input decoder or InputPort in this branch.
	go func() { defer close(done); var b [1]byte; _, _ = io.ReadFull(channel.stream, b[:]); cancel() }()
	defer func() {
		result = retireCursorPresentation(result, channel, service)
		<-done
	}()
	return wininput.RunProtectedReturnCursor(run, func() error {
		return authorizeWorkerOwners(run, channel, service)
	}, lease.CursorX, lease.CursorY, ready)
}
