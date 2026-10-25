package residentagent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/syncgate"
	"github.com/0disoft/krasis/internal/transport"
)

var errPrivilegedInput = errors.New("installed input channel unavailable")

type privilegedClient interface {
	application.InputPort
	Heartbeat(context.Context) error
	Close() error
}

type privilegedLeaseInput struct {
	mu                                      syncgate.Mutex
	client                                  privilegedClient
	identity                                application.LeaseIdentity
	cancel                                  context.CancelFunc
	done                                    chan struct{}
	connect                                 func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error)
	prepare                                 func(context.Context) error
	failed                                  func(error)
	event                                   func(string)
	expires                                 time.Time
	leaseExpires                            time.Time
	profile                                 context.Context
	dispose                                 func() error
	shutdownOnce                            sync.Once
	shutdownErr                             error
	closeErr                                error
	stopped                                 bool
	cursor                                  protectedReturnPresentation
	source                                  agent.SourceHook
	preparationStarted, preparationFinished time.Time
	entryWaiting                            atomic.Int32
	preparationCancel                       atomic.Pointer[preparationCancellation]
}

// Approval stays in the resident agent after the authenticated mTLS peer check.
// No privileged channel is opened for a rejected lease.
type privilegedAuthorizer struct {
	peer.LeaseAuthorizer
	input *privilegedLeaseInput
}

func (a privilegedAuthorizer) AuthorizeLease(ctx context.Context, p transport.AuthenticatedPeerContext, session, epoch string, now time.Time) (string, bool, error) {
	started := time.Now()
	id, accepted, err := a.LeaseAuthorizer.AuthorizeLease(ctx, p, session, epoch, now)
	handoff.Record(ctx, handoff.PeerAuthorize, time.Since(started))
	if err != nil || !accepted {
		return id, accepted, err
	}
	identity := application.LeaseIdentity{SessionID: session, LeaseID: id, Epoch: epoch}
	if err := a.input.open(ctx, identity, now); err != nil {
		return "", false, err
	}
	return id, true, nil
}

func (input *privilegedLeaseInput) open(ctx context.Context, identity application.LeaseIdentity, now time.Time) error {
	admissionStarted := time.Now()
	defer func() { input.observeHandoffTiming("admission", admissionStarted) }()
	if ctx == nil || ctx.Err() != nil || identity.SessionID == "" || identity.LeaseID == "" || identity.Epoch == "" {
		return errPrivilegedInput
	}
	input.entryWaiting.Add(1)
	defer input.entryWaiting.Add(-1)
	input.RestoreReturnCursor()
	started := time.Now()
	joined := input.finishPreparationForEntry(ctx)
	if err := input.mu.LockContext(ctx); err != nil {
		handoff.Record(ctx, handoff.OpenLock, time.Since(started))
		return errors.Join(errPrivilegedInput, err)
	}
	defer input.mu.Unlock()
	acquired := time.Now()
	handoff.Record(ctx, handoff.OpenLock, acquired.Sub(started))
	handoff.RecordPreparationLock(ctx, preparationLockOverlap(started, acquired, input.preparationStarted, input.preparationFinished))
	if input.stopped || input.client != nil || input.connect == nil || input.closeErr != nil {
		if input.event != nil {
			reason := "unconfigured"
			switch {
			case input.closeErr != nil:
				reason = "cleanup"
			case input.stopped:
				reason = "stopped"
			case input.client != nil:
				reason = "active"
			}
			input.event("installed_input_admission_blocked_" + reason)
		}
		if input.closeErr != nil {
			return errors.Join(errPrivilegedInput, input.closeErr)
		}
		return errPrivilegedInput
	}
	expires := now.Add(55 * time.Minute)
	if !input.expires.IsZero() && input.expires.Before(expires) {
		expires = input.expires
	}
	if !expires.After(now) {
		return errPrivilegedInput
	}
	if joined && input.event != nil {
		input.event("installed_input_preparation_joined")
	}
	handoff.Backend(ctx, "installed")
	started = time.Now()
	client, err := input.connect(ctx, identity, expires)
	handoff.Record(ctx, handoff.ConnectTotal, time.Since(started))
	// Connection completion can race cancellation after its final check. Retire
	// a late client through the existing failure path before publishing authority.
	if err == nil {
		err = ctx.Err()
	}
	if err != nil || client == nil {
		if client != nil {
			input.closeErr = errors.Join(input.closeErr, client.Close())
		}
		if input.event != nil {
			input.event("installed_input_connect_failed")
			input.event(inputworker.ConnectionFailureEvent(err))
			input.event(inputworker.ConnectionNativeFailureEvent(err))
		}
		if input.closeErr != nil {
			return errors.Join(errPrivilegedInput, err, input.closeErr)
		}
		if err != nil {
			return errors.Join(errPrivilegedInput, err)
		}
		return errPrivilegedInput
	}
	if observer, ok := client.(interface{ SetTimingObserver(func(string)) }); ok && input.event != nil {
		observer.SetTimingObserver(input.event)
	}
	if input.event != nil {
		input.event("installed_input_lease_ready")
	}
	run, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	input.client, input.identity, input.cancel, input.done = client, identity, cancel, done
	input.leaseExpires = expires
	input.profile = ctx
	go func() {
		defer close(done)
		ticker := time.NewTicker(250 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-run.Done():
				return
			case <-ticker.C:
				if err := client.Heartbeat(run); err != nil {
					if run.Err() == nil && input.failed != nil {
						if input.event != nil {
							input.event("installed_input_heartbeat_failed")
						}
						input.failed(errPrivilegedInput)
					}
					return
				}
			}
		}
	}()
	return nil
}

func (input *privilegedLeaseInput) close(intent *control.ReturnIntent) {
	input.mu.Lock()
	defer input.mu.Unlock()
	if intent != nil && (intent.SessionID != input.identity.SessionID || intent.Epoch != input.identity.Epoch) {
		return
	}
	client, cancel, done := input.client, input.cancel, input.done
	profile := input.profile
	input.profile = nil
	input.client, input.cancel, input.done = nil, nil, nil
	input.identity = application.LeaseIdentity{}
	started := time.Now()
	defer func() { handoff.Record(profile, handoff.InputClose, time.Since(started)) }()
	if cancel != nil {
		cancel()
	}
	if client != nil {
		input.closeErr = errors.Join(input.closeErr, client.Close())
	}
	if done != nil {
		<-done
	}
}

// A boundary return closes only its lease; installation pins survive reentry.
// Product connection shutdown waits for the client/heartbeat cleanup before
// releasing the shared pins, so the next update can replace installed files.
func (input *privilegedLeaseInput) shutdown() error {
	input.shutdownOnce.Do(func() {
		input.entryWaiting.Add(1) // Fence new hints even before stopped owns mu.
		defer input.entryWaiting.Add(-1)
		input.cancelPreparation()
		input.RestoreReturnCursor()
		input.mu.Lock()
		input.stopped = true
		input.mu.Unlock()
		input.close(nil)
		input.mu.Lock()
		input.shutdownErr = input.closeErr
		input.mu.Unlock()
		if input.dispose != nil {
			disposeErr := input.dispose()
			if input.shutdownErr == nil {
				input.shutdownErr = disposeErr
			} else {
				input.shutdownErr = errors.Join(input.shutdownErr, disposeErr)
			}
		}
	})
	return input.shutdownErr
}

func (input *privilegedLeaseInput) perform(ctx context.Context, operation func(privilegedClient) error) error {
	if ctx == nil {
		return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: errPrivilegedInput}
	}
	started := time.Now()
	if err := input.mu.LockContext(ctx); err != nil {
		handoff.Record(ctx, handoff.OuterLock, time.Since(started))
		return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: errors.Join(errPrivilegedInput, err)}
	}
	defer input.mu.Unlock()
	handoff.Record(ctx, handoff.OuterLock, time.Since(started))
	if input.client == nil || ctx.Err() != nil {
		return &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: errors.Join(errPrivilegedInput, ctx.Err())}
	}
	started = time.Now()
	err := operation(input.client)
	handoff.Record(ctx, handoff.ClientTotal, time.Since(started))
	return err
}
func (input *privilegedLeaseInput) Press(ctx context.Context, t control.PressedToken) error {
	return input.perform(ctx, func(p privilegedClient) error { return p.Press(ctx, t) })
}
func (input *privilegedLeaseInput) Release(ctx context.Context, t control.PressedToken) error {
	return input.perform(ctx, func(p privilegedClient) error { return p.Release(ctx, t) })
}
func (input *privilegedLeaseInput) Move(ctx context.Context, x, y int32) error {
	return input.perform(ctx, func(p privilegedClient) error { return p.Move(ctx, x, y) })
}
func (input *privilegedLeaseInput) Wheel(ctx context.Context, x, y int32) error {
	return input.perform(ctx, func(p privilegedClient) error { return p.Wheel(ctx, x, y) })
}

// Keep local cursor preparation and return tracking, but retire the worker only
// after runtime key-up recovery and a matching completed return/lease abort.
type privilegedBoundary struct {
	*guardedProductInput
	worker *privilegedLeaseInput
}

// The SYSTEM worker owns the crash ledger for delegated presses. Recording the
// same press in the ordinary crash guard would race its independent key-up.
type workerOwnedPressGuard struct{}

func (workerOwnedPressGuard) PreparePressContext(context.Context, control.PressedToken) (bool, error) {
	return false, nil
}
func (workerOwnedPressGuard) CancelPreparedPress(control.PressedToken) error { return nil }
func (workerOwnedPressGuard) Released(control.PressedToken) error            { return nil }
func (input *privilegedBoundary) DisarmReturn(intent control.ReturnIntent) {
	input.guardedProductInput.DisarmReturn(intent)
	input.worker.close(&intent)
}
