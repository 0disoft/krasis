package application

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/syncgate"
)

var (
	ErrRuntimeNotInbound = errors.New("session runtime is not inbound active")
	ErrRuntimeNoLiveness = errors.New("session runtime has no active peer liveness")
	ErrRuntimeInputHeld  = errors.New("session runtime has pressed input")
	ErrStaleReturnIntent = errors.New("return intent does not match the active inbound session")
)

// Match the protected worker's two-second I/O budget. Cooperative adapters must
// retire an operation before returning; no detached injection may outlive a
// lease. A native call that ignores cancellation is still joined synchronously.
const InputInjectionTimeout = 2 * time.Second
const InputFailureCleanupTimeout = 2 * time.Second

type LeaseIdentity struct {
	SessionID string
	LeaseID   string
	Epoch     string
}

type RuntimeSnapshot struct {
	Control           control.Snapshot
	InputStream       control.InputStream
	HasStream         bool
	OutboundStream    control.OutboundStream
	HasOutboundStream bool
	Liveness          control.PeerLiveness
	HasLiveness       bool
	PressedCount      int
}

type InputDisposition uint8

const (
	InputDispositionApplied InputDisposition = iota + 1
	InputDispositionRetiredDrained
	InputDispositionRejected
)

// Admission can be rejected before acquiring the runtime lock. These observers
// must be concurrency-safe, nonblocking counters rather than mutable I/O sinks.
type InputDispositionObserver interface {
	ObserveDestinationInputReceived()
	ObserveDestinationInputDisposition(InputDisposition)
}

// InputInjectionObserver measures adapter work separately from frame dispatch.
// Implementations on this path must not wait or perform I/O.
type InputInjectionObserver interface {
	ObserveDestinationInjectionStarted(time.Time)
	ObserveDestinationInjectionCompleted(time.Time)
}

// SessionRuntime is the single serialized owner of one peer session's mutable
// control, ordering, liveness, and pressed-input state.
type SessionRuntime struct {
	watch          atomic.Pointer[LeaseWatch]
	mu             syncgate.Mutex
	control        control.Snapshot
	stream         control.InputStream
	retiredInbound control.InputStream
	outbound       control.OutboundStream
	liveness       control.PeerLiveness
	pressed        control.PressedState
	executor       *SafetyEffectExecutor
	input          InputPort
	inputObserver  InputDispositionObserver
}

func NewSessionRuntime(delegate EffectExecutor, input InputPort) (*SessionRuntime, error) {
	return NewSessionRuntimeObserved(delegate, input, nil)
}

func NewSessionRuntimeObserved(
	delegate EffectExecutor,
	input InputPort,
	observer InputDispositionObserver,
) (*SessionRuntime, error) {
	if delegate == nil || input == nil {
		return nil, errors.New("effect delegate and input port are required")
	}
	runtime := &SessionRuntime{
		control:       control.InitialSnapshot(),
		input:         input,
		inputObserver: observer,
	}
	executor, err := NewSafetyEffectExecutor(delegate, &runtime.pressed, input)
	if err != nil {
		return nil, err
	}
	runtime.executor = executor
	return runtime, nil
}

func (runtime *SessionRuntime) Snapshot() RuntimeSnapshot {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	return runtime.snapshotLocked()
}

// SnapshotContext never reports a guessed state when another operation owns
// the runtime beyond this caller's recovery budget.
func (runtime *SessionRuntime) SnapshotContext(ctx context.Context) (RuntimeSnapshot, error) {
	if err := runtime.mu.LockContext(ctx); err != nil {
		return RuntimeSnapshot{}, err
	}
	defer runtime.mu.Unlock()
	return runtime.snapshotLocked(), nil
}

func (runtime *SessionRuntime) Dispatch(
	ctx context.Context,
	event control.Event,
	observedAt time.Time,
) (ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if event.Kind == control.EventInboundLeaseGranted || event.Kind == control.EventInboundLeasePrepared ||
		event.Kind == control.EventInboundLeaseCommitted || event.Kind == control.EventInboundLeaseAborted {
		return ControlOutcome{}, errors.New("inbound lease transitions require dedicated runtime methods")
	}
	if event.Kind == control.EventOutboundIntentDetected {
		return ControlOutcome{}, errors.New("outbound intents require BeginOutbound")
	}
	if event.Kind == control.EventOutboundLeaseGranted {
		return ControlOutcome{}, errors.New("outbound lease grants require GrantOutbound")
	}
	if event.Kind == control.EventOutboundLeaseRejected {
		return ControlOutcome{}, errors.New("outbound lease rejections require RejectOutbound")
	}
	if event.Kind == control.EventOutboundLeaseAborted {
		return ControlOutcome{}, errors.New("outbound lease aborts require AbortOutbound")
	}
	if event.Kind == control.EventReleaseAllSucceeded || event.Kind == control.EventReleaseAllFailed {
		return ControlOutcome{}, errors.New("ReleaseAll completion events are runtime-internal")
	}
	outcome, err := HandleControlEvent(ctx, runtime.control, event, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	if err != nil {
		return outcome, err
	}
	return outcome, nil
}

func (runtime *SessionRuntime) BeginOutbound(
	ctx context.Context,
	sessionID string,
	epoch string,
) (ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	stream, err := control.NewOutboundStream(sessionID, epoch)
	if err != nil {
		return ControlOutcome{}, err
	}
	outcome, err := HandleControlEvent(ctx, runtime.control, control.Event{
		Kind:      control.EventOutboundIntentDetected,
		SessionID: sessionID,
	}, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	if err != nil {
		return outcome, err
	}
	if outcome.Snapshot.State != control.StatePending {
		return runtime.faultLocked(ctx, errors.New("outbound intent did not produce pending state"))
	}
	runtime.outbound = stream
	return outcome, nil
}

func (runtime *SessionRuntime) GrantOutbound(
	ctx context.Context,
	sessionID string,
	leaseID string,
	epoch string,
	observedAt time.Time,
) (ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.control.State != control.StatePending || runtime.outbound.SessionID == "" {
		return ControlOutcome{}, errors.New("outbound grant requires a pending outbound stream")
	}
	if runtime.outbound.SessionID != sessionID || runtime.outbound.Epoch != epoch {
		return runtime.faultLocked(ctx, errors.New("outbound grant does not match pending session and epoch"))
	}
	if leaseID == "" {
		return runtime.faultLocked(ctx, errors.New("outbound grant requires a lease identifier"))
	}
	liveness, err := control.NewPeerLiveness(sessionID, observedAt)
	if err != nil {
		return runtime.faultLocked(ctx, err)
	}
	outcome, err := HandleControlEvent(ctx, runtime.control, control.Event{
		Kind:      control.EventOutboundLeaseGranted,
		SessionID: sessionID,
		LeaseID:   leaseID,
	}, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	if err != nil {
		return outcome, err
	}
	runtime.liveness = liveness
	return outcome, nil
}

func (runtime *SessionRuntime) NextOutboundEnvelope(ctx context.Context) (control.InputEnvelope, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.control.State != control.StateOutboundActive || runtime.outbound.SessionID == "" {
		return control.InputEnvelope{}, errors.New("outbound input requires an outbound-active stream")
	}
	envelope, next, err := runtime.outbound.Next()
	if err != nil {
		_, faultErr := runtime.faultLocked(ctx, err)
		return control.InputEnvelope{}, faultErr
	}
	runtime.outbound = next
	return envelope, nil
}

func (runtime *SessionRuntime) RejectOutbound(
	ctx context.Context,
	sessionID string,
	epoch string,
) (ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.control.State != control.StatePending || runtime.outbound.SessionID == "" {
		return ControlOutcome{}, errors.New("outbound rejection requires a pending outbound stream")
	}
	if runtime.outbound.SessionID != sessionID || runtime.outbound.Epoch != epoch {
		return runtime.faultLocked(ctx, errors.New("outbound rejection does not match pending session and epoch"))
	}
	outcome, err := HandleControlEvent(ctx, runtime.control, control.Event{
		Kind:      control.EventOutboundLeaseRejected,
		SessionID: sessionID,
	}, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	return outcome, err
}

func (runtime *SessionRuntime) AbortOutbound(
	ctx context.Context,
	sessionID string,
	leaseID string,
	epoch string,
) (ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.control.State != control.StatePending && runtime.control.State != control.StateOutboundActive {
		return ControlOutcome{}, errors.New("outbound abort requires a pending or active outbound stream")
	}
	if runtime.outbound.SessionID != sessionID || runtime.outbound.Epoch != epoch || leaseID == "" {
		return runtime.faultLocked(ctx, errors.New("outbound abort does not match session, lease, and epoch"))
	}
	if runtime.control.State == control.StateOutboundActive && runtime.control.LeaseID != leaseID {
		return runtime.faultLocked(ctx, errors.New("outbound abort does not match the active lease"))
	}
	outcome, err := HandleControlEvent(ctx, runtime.control, control.Event{
		Kind:      control.EventOutboundLeaseAborted,
		SessionID: sessionID,
		LeaseID:   leaseID,
	}, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	return outcome, err
}

func (runtime *SessionRuntime) RevokePeer(
	ctx context.Context,
	sessionID string,
	leaseID string,
	epoch string,
) (ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if runtime.control.State != control.StateInboundActive && runtime.control.State != control.StateOutboundActive {
		return ControlOutcome{}, errors.New("peer revocation requires an active lease")
	}
	currentEpoch := runtime.stream.Epoch
	if runtime.control.State == control.StateOutboundActive {
		currentEpoch = runtime.outbound.Epoch
	}
	if runtime.control.SessionID != sessionID || runtime.control.LeaseID != leaseID || currentEpoch != epoch {
		return runtime.faultLocked(ctx, errors.New("peer revocation does not match active session, lease, and epoch"))
	}
	outcome, err := HandleControlEvent(ctx, runtime.control, control.Event{Kind: control.EventRevokeRequested}, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	return outcome, err
}

func (runtime *SessionRuntime) RevokeLocalReturn(
	ctx context.Context,
	intent control.ReturnIntent,
) (LeaseIdentity, ControlOutcome, error) {
	return runtime.revokeReturn(ctx, intent, "")
}

// A planned retirement releases held inputs under the same runtime lock before
// revoking the exact lease. A failed release keeps the runtime faulted.
func (runtime *SessionRuntime) RetireInbound(ctx context.Context, identity LeaseIdentity) (LeaseIdentity, ControlOutcome, error) {
	if identity.LeaseID == "" {
		return LeaseIdentity{}, ControlOutcome{}, ErrStaleReturnIntent
	}
	return runtime.revokeReturn(ctx, control.ReturnIntent{SessionID: identity.SessionID, Epoch: identity.Epoch}, identity.LeaseID)
}

func (runtime *SessionRuntime) revokeReturn(ctx context.Context, intent control.ReturnIntent, leaseID string) (LeaseIdentity, ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()
	if err := intent.Validate(); err != nil {
		return LeaseIdentity{}, ControlOutcome{}, err
	}
	if runtime.control.State != control.StateInboundActive || runtime.stream.SessionID == "" {
		return LeaseIdentity{}, ControlOutcome{}, ErrRuntimeNotInbound
	}
	if runtime.control.SessionID != intent.SessionID || runtime.stream.Epoch != intent.Epoch || (leaseID != "" && runtime.control.LeaseID != leaseID) {
		return LeaseIdentity{}, ControlOutcome{}, ErrStaleReturnIntent
	}
	if leaseID == "" && runtime.pressed.Count() != 0 {
		return LeaseIdentity{}, ControlOutcome{}, ErrRuntimeInputHeld
	}
	identity := LeaseIdentity{
		SessionID: runtime.control.SessionID,
		LeaseID:   runtime.control.LeaseID,
		Epoch:     runtime.stream.Epoch,
	}
	retired := runtime.stream
	outcome, err := HandleControlEvent(ctx, runtime.control, control.Event{Kind: control.EventRevokeRequested}, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	if err == nil && outcome.Snapshot.State == control.StateLocal {
		runtime.retiredInbound = retired
	}
	return identity, outcome, err
}

func (runtime *SessionRuntime) GrantInbound(
	ctx context.Context,
	event control.Event,
	epoch string,
	observedAt time.Time,
) (ControlOutcome, error) {
	if event.Kind != control.EventInboundLeaseGranted {
		return ControlOutcome{}, errors.New("GrantInbound requires an inbound lease grant event")
	}
	if _, err := runtime.PrepareInbound(ctx, event.SessionID, event.LeaseID, epoch); err != nil {
		return ControlOutcome{}, err
	}
	return runtime.CommitInbound(ctx, event.SessionID, event.LeaseID, epoch, observedAt)
}

func (runtime *SessionRuntime) PrepareInbound(
	ctx context.Context,
	sessionID string,
	leaseID string,
	epoch string,
) (ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	stream, err := control.NewInputStream(sessionID, epoch)
	if err != nil {
		return ControlOutcome{}, err
	}
	outcome, err := HandleControlEvent(ctx, runtime.control, control.Event{
		Kind:      control.EventInboundLeasePrepared,
		SessionID: sessionID,
		LeaseID:   leaseID,
	}, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	if err != nil {
		return outcome, err
	}
	if outcome.Snapshot.State != control.StateInboundPending {
		return runtime.faultLocked(ctx, errors.New("inbound preparation did not produce inbound-pending state"))
	}
	runtime.stream = stream
	runtime.retiredInbound = control.InputStream{}
	return outcome, nil
}

func (runtime *SessionRuntime) CommitInbound(
	ctx context.Context,
	sessionID string,
	leaseID string,
	epoch string,
	observedAt time.Time,
) (ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.control.State != control.StateInboundPending || runtime.stream.SessionID == "" {
		return ControlOutcome{}, errors.New("inbound commit requires a prepared inbound stream")
	}
	if runtime.control.SessionID != sessionID || runtime.control.LeaseID != leaseID || runtime.stream.Epoch != epoch {
		return runtime.faultLocked(ctx, errors.New("inbound commit does not match prepared session, lease, and epoch"))
	}
	liveness, err := control.NewPeerLiveness(sessionID, observedAt)
	if err != nil {
		return runtime.faultLocked(ctx, err)
	}
	outcome, err := HandleControlEvent(ctx, runtime.control, control.Event{
		Kind:      control.EventInboundLeaseCommitted,
		SessionID: sessionID,
		LeaseID:   leaseID,
	}, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	if err != nil {
		return outcome, err
	}
	if outcome.Snapshot.State != control.StateInboundActive {
		return runtime.faultLocked(ctx, errors.New("inbound commit did not produce inbound-active state"))
	}
	runtime.liveness = liveness
	return outcome, nil
}

func (runtime *SessionRuntime) AbortInbound(
	ctx context.Context,
	sessionID string,
	leaseID string,
	epoch string,
) (ControlOutcome, error) {
	runtime.mu.Lock()
	defer runtime.mu.Unlock()

	if runtime.control.State != control.StateInboundPending || runtime.stream.SessionID == "" {
		return ControlOutcome{}, errors.New("inbound abort requires a prepared inbound stream")
	}
	if runtime.control.SessionID != sessionID || runtime.control.LeaseID != leaseID || runtime.stream.Epoch != epoch {
		return runtime.faultLocked(ctx, errors.New("inbound abort does not match prepared session, lease, and epoch"))
	}
	outcome, err := HandleControlEvent(ctx, runtime.control, control.Event{
		Kind:      control.EventInboundLeaseAborted,
		SessionID: sessionID,
		LeaseID:   leaseID,
	}, runtime.executor)
	runtime.applyOutcomeLocked(outcome)
	return outcome, err
}

func (runtime *SessionRuntime) HandlePressedInput(
	ctx context.Context,
	envelope control.InputEnvelope,
	token control.PressedToken,
	pressed bool,
) error {
	if err := token.Validate(); err != nil {
		runtime.mu.Lock()
		defer runtime.mu.Unlock()
		if runtime.control.State != control.StateInboundActive || runtime.stream.SessionID == "" {
			return ErrRuntimeNotInbound
		}
		_, faultErr := runtime.faultLocked(ctx, err)
		return faultErr
	}
	kind := control.InputKeyUp
	switch token.Kind {
	case control.PressedKeyboardKey:
		if pressed {
			kind = control.InputKeyDown
		}
	case control.PressedMouseButton:
		kind = control.InputButtonUp
		if pressed {
			kind = control.InputButtonDown
		}
	}
	return runtime.HandleInputAction(ctx, envelope, control.InputAction{Kind: kind, Code: token.Code})
}

func (runtime *SessionRuntime) HandleInputAction(
	ctx context.Context,
	envelope control.InputEnvelope,
	action control.InputAction,
) error {
	return runtime.handleInputAction(ctx, envelope, action, nil, 0)
}

func (runtime *SessionRuntime) handleInputAction(
	ctx context.Context,
	envelope control.InputEnvelope,
	action control.InputAction,
	now func() time.Time,
	timeout time.Duration,
) (returnErr error) {
	disposition := InputDispositionRejected
	if runtime.inputObserver != nil {
		runtime.inputObserver.ObserveDestinationInputReceived()
		defer func() {
			runtime.inputObserver.ObserveDestinationInputDisposition(disposition)
		}()
	}
	// Admission and adapter work share one budget. A canceled lock waiter
	// cannot consume a stream sequence or call an input adapter later.
	injectionCtx, stopInjection := context.WithTimeout(ctx, InputInjectionTimeout)
	defer stopInjection()
	if err := runtime.mu.LockContext(injectionCtx); err != nil {
		return err
	}
	defer runtime.mu.Unlock()

	if runtime.control.State != control.StateInboundActive || runtime.stream.SessionID == "" {
		if runtime.canDrainRetiredLocked() {
			if err := action.Validate(); err != nil {
				return err
			}
			next, err := runtime.retiredInbound.Accept(envelope)
			if err != nil {
				return err
			}
			runtime.retiredInbound = next
			disposition = InputDispositionRetiredDrained
			return nil
		}
		if runtime.control.State != control.StateLocal {
			_, faultErr := runtime.faultLocked(ctx, ErrRuntimeNotInbound)
			return faultErr
		}
		return ErrRuntimeNotInbound
	}
	if err := action.Validate(); err != nil {
		_, faultErr := runtime.faultLocked(ctx, err)
		return faultErr
	}
	next, outcome, accepted, err := HandleInputEnvelope(
		ctx,
		runtime.control,
		runtime.stream,
		envelope,
		runtime.executor,
	)
	runtime.applyOutcomeLocked(outcome)
	if err != nil {
		return err
	}
	if !accepted {
		return errors.New("input envelope was not accepted")
	}
	runtime.stream = next
	progress, admittedAt, err := runtime.inputLivenessLocked(injectionCtx, now, timeout)
	if err != nil {
		return err
	}

	err = runtime.injectAction(injectionCtx, action)
	if expired := injectionCtx.Err(); expired != nil {
		// A late successful return cannot turn an expired operation into a
		// receipt. Keep its press ledger and recover instead of retrying input.
		err = errors.Join(err, expired)
	}
	stopInjection()
	if err == nil {
		if now != nil {
			runtime.liveness = progress
			runtime.watch.Store(&LeaseWatch{State: runtime.control.State, SessionID: runtime.control.SessionID, Progress: admittedAt})
		}
		disposition = InputDispositionApplied
		return nil
	}
	// An expired injection/session context must not prevent key-up recovery.
	// The runtime remains the serialized owner until this cleanup completes.
	cleanupCtx, stopCleanup := context.WithTimeout(context.WithoutCancel(ctx), InputFailureCleanupTimeout)
	defer stopCleanup()
	_, faultErr := runtime.faultLocked(cleanupCtx, err)
	return faultErr
}

func (runtime *SessionRuntime) injectAction(ctx context.Context, action control.InputAction) (err error) {
	if observer, ok := runtime.inputObserver.(InputInjectionObserver); ok {
		observer.ObserveDestinationInjectionStarted(time.Now())
		defer func() { observer.ObserveDestinationInjectionCompleted(time.Now()) }()
	}
	if token, pressed, discrete := action.PressedToken(); discrete {
		if pressed {
			err = InjectPress(ctx, &runtime.pressed, runtime.input, token)
		} else {
			_, err = InjectRelease(ctx, &runtime.pressed, runtime.input, token)
		}
	} else if action.Kind == control.InputPointerMove {
		err = runtime.input.Move(ctx, action.DeltaX, action.DeltaY)
	} else {
		err = runtime.input.Wheel(ctx, action.DeltaX, action.DeltaY)
	}
	return err
}

func (runtime *SessionRuntime) ObserveHeartbeat(
	ctx context.Context,
	sessionID string,
	observedAt time.Time,
) error {
	if err := runtime.mu.LockContext(ctx); err != nil {
		return err
	}
	defer runtime.mu.Unlock()
	if runtime.canDrainRetiredLocked() && runtime.retiredInbound.SessionID == sessionID {
		return nil
	}

	if runtime.control.State != control.StateInboundActive && runtime.control.State != control.StateOutboundActive {
		return ErrRuntimeNoLiveness
	}
	next, err := runtime.liveness.Observe(sessionID, observedAt)
	if err != nil {
		_, faultErr := runtime.faultLocked(ctx, err)
		return faultErr
	}
	runtime.liveness = next
	runtime.publishLeaseWatchLocked(true)
	return nil
}

func (runtime *SessionRuntime) canDrainRetiredLocked() bool {
	if runtime.retiredInbound.SessionID == "" {
		return false
	}
	switch runtime.control.State {
	case control.StateLocal:
		return true
	case control.StatePending, control.StateOutboundActive:
		return runtime.retiredInbound.SessionID != runtime.control.SessionID
	default:
		return false
	}
}

func (runtime *SessionRuntime) CheckLiveness(
	ctx context.Context,
	now time.Time,
	timeout time.Duration,
) (bool, error) {
	if err := runtime.mu.LockContext(ctx); err != nil {
		return false, err
	}
	defer runtime.mu.Unlock()

	if runtime.control.State != control.StateInboundActive && runtime.control.State != control.StateOutboundActive {
		return false, ErrRuntimeNoLiveness
	}
	outcome, faulted, err := HandleLiveness(
		ctx,
		runtime.control,
		runtime.liveness,
		now,
		timeout,
		runtime.executor,
	)
	if faulted {
		runtime.applyOutcomeLocked(outcome)
	}
	return faulted, err
}

func (runtime *SessionRuntime) Fail(ctx context.Context, cause error) error {
	if cause == nil {
		cause = errors.New("peer session failed")
	}
	if err := runtime.mu.LockContext(ctx); err != nil {
		return errors.Join(cause, err)
	}
	defer runtime.mu.Unlock()
	if runtime.control.State == control.StateLocal {
		return cause
	}
	_, err := runtime.faultLocked(ctx, cause)
	return err
}

func (runtime *SessionRuntime) faultLocked(ctx context.Context, cause error) (ControlOutcome, error) {
	outcome, faultErr := HandleControlEvent(
		ctx,
		runtime.control,
		control.Event{Kind: control.EventFaultDetected},
		runtime.executor,
	)
	runtime.applyOutcomeLocked(outcome)
	if faultErr == nil {
		return outcome, cause
	}
	return outcome, errors.Join(cause, faultErr)
}

func (runtime *SessionRuntime) applyOutcomeLocked(outcome ControlOutcome) {
	if outcome.Snapshot.State == "" {
		return
	}
	runtime.control = outcome.Snapshot
	runtime.publishLeaseWatchLocked(false)
	if runtime.control.State != control.StateInboundPending && runtime.control.State != control.StateInboundActive {
		runtime.stream = control.InputStream{}
	}
	if runtime.control.State != control.StatePending && runtime.control.State != control.StateOutboundActive {
		runtime.outbound = control.OutboundStream{}
	}
	if runtime.control.State != control.StateInboundActive && runtime.control.State != control.StateOutboundActive {
		runtime.liveness = control.PeerLiveness{}
	}
}

func (runtime *SessionRuntime) snapshotLocked() RuntimeSnapshot {
	return RuntimeSnapshot{
		Control:           runtime.control,
		InputStream:       runtime.stream,
		HasStream:         runtime.stream.SessionID != "",
		OutboundStream:    runtime.outbound,
		HasOutboundStream: runtime.outbound.SessionID != "",
		Liveness:          runtime.liveness,
		HasLiveness:       runtime.liveness.SessionID != "",
		PressedCount:      runtime.pressed.Count(),
	}
}

func (snapshot RuntimeSnapshot) String() string {
	return fmt.Sprintf(
		"state=%s version=%d inbound_stream=%t outbound_stream=%t liveness=%t pressed=%d",
		snapshot.Control.State,
		snapshot.Control.Version,
		snapshot.HasStream,
		snapshot.HasOutboundStream,
		snapshot.HasLiveness,
		snapshot.PressedCount,
	)
}
