package wininput

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

const MaxSourceQueueCapacity = 65536

const SourceCleanupTimeout = 2 * time.Second

var (
	ErrSourceQueueOverflow     = errors.New("source input queue overflow")
	ErrSourceSession           = errors.New("source input session failed")
	ErrSourceCleanupIncomplete = errors.New("source cleanup completion unconfirmed")
)

type SourceSink interface {
	// SendAction must check isCurrent after acquiring its semantic write lock,
	// before assigning the action to an outbound session.
	SendAction(context.Context, control.InputAction, func() bool) error
	Interrupt(error) error
	Fail(context.Context, error) error
}

type SourceActivation interface {
	Activate() error
	Deactivate()
	Active() bool
}

// Optional activation diagnostics; existing adapters keep SourceActivation.
type SourceProfiledActivation interface {
	SourceActivation
	ActivateProfile(context.Context) error
}

// Optional return/rollback diagnostics keep the same attempt on the native owner.
type SourceProfiledDeactivation interface {
	SourceActivation
	DeactivateProfile(context.Context)
}

type SourceWheelOwnership interface {
	SourceActivation
	OwnsWheel() bool
}

type SourcePipelineObserver interface {
	ObserveWheelHook()
	ObserveWheelSynthesized()
	ObserveWheelRaw()
	ObserveWheelWindow()
	ObserveWheelCaptureActivated()
	ObserveWheelCaptureDeactivated()
	ObserveWheelForegroundFallback(result SourceForegroundFallbackResult)
	ObserveWheelFocusRestoreFallback(result SourceFocusRestoreFallbackResult)
	ObserveWheelFocusRestore(result SourceFocusRestoreResult)
	ObserveWheelPostHideFocus(result SourcePostHideFocusResult)
	ObserveActiveHookMouseMove()
	ObserveActiveHookMouseButton()
	ObserveWheelMapped()
	ObserveWheelQueue(SourceQueueOfferResult)
	ObserveSourceQueueEnqueued(depth uint64)
	ObserveSourceQueueDequeued(residence time.Duration)
	ObserveSourceQueueDiscardedStale()
}

// SourceQueueObserver receives aggregate queue events without physical input data.
// Implementations on a hook callback must not wait or perform I/O.
type SourceQueueObserver interface {
	ObserveSourceQueueEnqueued(depth uint64)
	ObserveSourceQueueDequeued(residence time.Duration)
	ObserveSourceQueueDiscardedStale()
}

type SourceQueueCoalescedObserver interface {
	ObserveSourceQueueCoalesced()
}

type SourceForegroundFallbackResult uint8

const (
	SourceForegroundFallbackSucceeded SourceForegroundFallbackResult = iota + 1
	SourceForegroundFallbackAttachFailed
	SourceForegroundFallbackDetachFailed
	SourceForegroundFallbackAPIRejected
	SourceForegroundFallbackVerificationLost
)

type SourceFocusRestoreFallbackResult uint8

const (
	SourceFocusRestoreFallbackSucceeded SourceFocusRestoreFallbackResult = iota + 1
	SourceFocusRestoreFallbackAttachFailed
	SourceFocusRestoreFallbackDetachFailed
	SourceFocusRestoreFallbackAPIRejected
	SourceFocusRestoreFallbackVerificationLost
)

type SourceFocusRestoreResult uint8

const (
	SourceFocusRestoreSkipped SourceFocusRestoreResult = iota + 1
	SourceFocusRestoreSucceeded
	SourceFocusRestoreAPIRejected
	SourceFocusRestoreVerificationLost
)

type SourcePostHideFocusResult uint8

const (
	SourcePostHideFocusRecovered SourcePostHideFocusResult = iota + 1
	SourcePostHideFocusUnavailable
	SourcePostHideFocusCaptureRetained
)

type SourceQueueOfferResult uint8

const (
	SourceQueueAccepted SourceQueueOfferResult = iota + 1
	SourceQueueRejectedInactive
	SourceQueueRejectedInvalid
	SourceQueueRejectedOverflow
)

type queuedAction struct {
	generation uint64
	action     control.InputAction
	queuedAt   time.Time
}

type SourceSession struct {
	sink             SourceSink
	observer         SourcePipelineObserver
	queueObserver    SourceQueueObserver
	cleanup          time.Duration
	cleanupState     atomic.Uint32
	queue            chan *queuedAction
	queueDepth       atomic.Int64
	queueMu          sync.Mutex
	pendingMove      *queuedAction
	faults           chan error
	yields           chan uint64
	yieldPending     uint64 // stateMu; prevents a new lease until local-return cleanup finishes.
	now              func() time.Time
	started          atomic.Bool
	terminal         atomic.Bool
	stateMu          sync.RWMutex
	physicalMu       sync.Mutex
	physicalHeld     map[physicalPress]struct{}
	physicalCheck    func() bool
	physicalRevision uint64 // physicalMu; invalidates native inspection across local input/owner changes.
	capturePaused    bool   // physicalMu; native owner replacement fences new grants.
	active           bool
	generation       uint64
	faultSent        atomic.Bool
}

func NewSourceSession(capacity int, sink SourceSink) (*SourceSession, error) {
	return NewSourceSessionObserved(capacity, sink, nil)
}

func NewSourceSessionObserved(capacity int, sink SourceSink, observer SourcePipelineObserver) (*SourceSession, error) {
	return NewSourceSessionWithObservers(capacity, sink, observer, nil)
}

func NewSourceSessionWithObservers(capacity int, sink SourceSink, observer SourcePipelineObserver, queueObserver SourceQueueObserver) (*SourceSession, error) {
	if capacity <= 0 || capacity > MaxSourceQueueCapacity {
		return nil, fmt.Errorf("source queue capacity must be between 1 and %d", MaxSourceQueueCapacity)
	}
	if sink == nil {
		return nil, errors.New("source sink is required")
	}
	return &SourceSession{
		sink:          sink,
		observer:      observer,
		queueObserver: queueObserver,
		cleanup:       SourceCleanupTimeout,
		queue:         make(chan *queuedAction, capacity),
		faults:        make(chan error, 1),
		yields:        make(chan uint64, 1),
		now:           time.Now,
	}, nil
}

func (session *SourceSession) Activate() error {
	if session == nil || !session.started.Load() || session.terminal.Load() {
		return errors.New("source session worker is not running")
	}
	session.physicalMu.Lock()
	if session.capturePaused {
		session.physicalMu.Unlock()
		return errors.New("source capture is being prepared")
	}
	if len(session.physicalHeld) != 0 {
		session.physicalMu.Unlock()
		return errors.New("physical input is held during source activation")
	}
	check, revision := session.physicalCheck, session.physicalRevision
	session.physicalMu.Unlock()
	// Native queries may yield. Never make the low-level hook wait for them.
	// The final ledger/revision check rejects even a complete local down/up or
	// native-owner replacement that occurred while this inspection was running.
	if check != nil && check() {
		return errors.New("physical input is held during source activation")
	}
	session.physicalMu.Lock()
	defer session.physicalMu.Unlock()
	if session.capturePaused || len(session.physicalHeld) != 0 ||
		revision != session.physicalRevision || revision == ^uint64(0) {
		return errors.New("physical input changed during source activation")
	}
	session.stateMu.Lock()
	defer session.stateMu.Unlock()
	if session.terminal.Load() || !session.started.Load() {
		return errors.New("source session worker has stopped")
	}
	if session.yieldPending != 0 {
		return errors.New("source local return is pending")
	}
	if session.active {
		return errors.New("source session is already active")
	}
	session.generation++
	session.active = true
	return nil
}

// Retire permanently rejects late activation from the old lease generation.
func (session *SourceSession) Retire() {
	if session == nil {
		return
	}
	session.terminal.Store(true)
	session.Deactivate()
}

func (session *SourceSession) Deactivate() {
	if session == nil {
		return
	}
	session.stateMu.Lock()
	session.active = false
	session.generation++
	session.stateMu.Unlock()
}

func (session *SourceSession) Active() bool {
	if session == nil || session.terminal.Load() {
		return false
	}
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	return session.active && !session.terminal.Load()
}

// Offer is safe for a low-level hook callback. It never blocks. It returns true
// only when the physical event was accepted for the current outbound generation
// and may therefore be suppressed locally.
func (session *SourceSession) Offer(action control.InputAction) bool {
	if session == nil || session.terminal.Load() {
		return false
	}
	session.stateMu.RLock()
	if !session.active || session.terminal.Load() {
		session.stateMu.RUnlock()
		session.observeWheelQueue(action, SourceQueueRejectedInactive)
		return false
	}
	if err := action.Validate(); err != nil {
		session.stateMu.RUnlock()
		session.abort(errors.Join(ErrSourceSession, err))
		session.observeWheelQueue(action, SourceQueueRejectedInvalid)
		return false
	}
	generation := session.generation
	queuedAt := session.now()
	session.queueMu.Lock()
	if action.Kind == control.InputPointerMove && session.pendingMove != nil && session.pendingMove.generation == generation {
		pending := session.pendingMove
		dx := int64(pending.action.DeltaX) + int64(action.DeltaX)
		dy := int64(pending.action.DeltaY) + int64(action.DeltaY)
		if (dx != 0 || dy != 0) && dx >= -control.MaxInputDelta && dx <= control.MaxInputDelta && dy >= -control.MaxInputDelta && dy <= control.MaxInputDelta {
			pending.action.DeltaX = int32(dx)
			pending.action.DeltaY = int32(dy)
			session.queueMu.Unlock()
			session.stateMu.RUnlock()
			if observer, ok := session.queueObserver.(SourceQueueCoalescedObserver); ok {
				observer.ObserveSourceQueueCoalesced()
			}
			return true
		}
	}
	// Reserve capacity before splitting: no partial frame may be admitted on
	// overload. Each axis retains its total and discrete inputs keep their order.
	count := deltaFrames(action)
	if count <= cap(session.queue)-len(session.queue) {
		var depth int64
		for i := 0; i < count; i++ {
			part := action
			if action.Kind == control.InputPointerMove || action.Kind == control.InputWheel {
				part.DeltaX = max(-control.MaxInputDelta, min(control.MaxInputDelta, action.DeltaX))
				part.DeltaY = max(-control.MaxInputDelta, min(control.MaxInputDelta, action.DeltaY))
				action.DeltaX -= part.DeltaX
				action.DeltaY -= part.DeltaY
			}
			item := &queuedAction{generation: generation, action: part, queuedAt: queuedAt}
			depth = session.queueDepth.Add(1)
			session.queue <- item
			if action.Kind == control.InputPointerMove {
				session.pendingMove = item
			} else {
				session.pendingMove = nil
			}
		}
		session.queueMu.Unlock()
		session.stateMu.RUnlock()
		if session.observer != nil {
			session.observer.ObserveSourceQueueEnqueued(uint64(depth))
		}
		if session.queueObserver != nil {
			session.queueObserver.ObserveSourceQueueEnqueued(uint64(depth))
		}
		session.observeWheelQueue(action, SourceQueueAccepted)
		return true
	} else {
		session.queueMu.Unlock()
		session.stateMu.RUnlock()
		session.abort(ErrSourceQueueOverflow)
		session.observeWheelQueue(action, SourceQueueRejectedOverflow)
		return false
	}
}

func deltaFrames(action control.InputAction) int {
	if action.Kind != control.InputPointerMove && action.Kind != control.InputWheel {
		return 1
	}
	x, y := int64(action.DeltaX), int64(action.DeltaY)
	if x < 0 {
		x = -x
	}
	if y < 0 {
		y = -y
	}
	return int((max(x, y) + control.MaxInputDelta - 1) / control.MaxInputDelta)
}

func (session *SourceSession) observeWheelQueue(action control.InputAction, result SourceQueueOfferResult) {
	if action.Kind == control.InputWheel && session.observer != nil {
		session.observer.ObserveWheelQueue(result)
	}
}

func (session *SourceSession) Abort(cause error) {
	if session == nil {
		return
	}
	if cause == nil {
		cause = ErrSourceSession
	}
	session.abort(cause)
}

func (session *SourceSession) Run(ctx context.Context) error {
	return session.RunWithReady(ctx, nil)
}

func (session *SourceSession) RunWithReady(ctx context.Context, ready func()) error {
	if session == nil || session.sink == nil {
		return errors.New("source session is not initialized")
	}
	if ctx == nil {
		return errors.New("source session context is required")
	}
	if session.terminal.Load() {
		// Retirement can win the scheduling race before this worker starts.
		// A canceled first start owns no native resources and is normal shutdown.
		if err := ctx.Err(); err != nil {
			return err
		}
		return errors.New("source session worker cannot be restarted")
	}
	if !session.started.CompareAndSwap(false, true) {
		return errors.New("source session worker is already running")
	}
	if ready != nil {
		ready()
	}
	defer session.started.Store(false)
	defer session.terminal.Store(true)
	defer session.Deactivate()
	sendCtx, cancelSend := context.WithCancel(ctx)
	defer cancelSend()
	senderDone := make(chan error, 1)
	go func() { senderDone <- session.runSender(sendCtx) }()
	for {
		select {
		case <-ctx.Done():
			// A hook watchdog fences suppression before canceling its owners.
			// Cancellation must not hide that queued failure and skip Fail.
			select {
			case cause := <-session.faults:
				return session.stopSender(ctx, cancelSend, senderDone, cause, true)
			default:
			}
			return session.stopSender(ctx, cancelSend, senderDone, ctx.Err(), session.Active())
		case cause := <-session.faults:
			return session.stopSender(ctx, cancelSend, senderDone, cause, true)
		case cause := <-senderDone:
			if cause == nil {
				cause = errors.New("source sender stopped unexpectedly")
			}
			return session.cleanupSession(ctx, nil, nil, cause, true)
		}
	}
}

func (session *SourceSession) runSender(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case queued := <-session.queue:
			session.queueMu.Lock()
			if session.pendingMove == queued {
				session.pendingMove = nil
			}
			item := *queued
			session.queueMu.Unlock()
			session.queueDepth.Add(-1)
			residence := session.now().Sub(item.queuedAt)
			if residence < 0 {
				residence = 0
			}
			if session.observer != nil {
				session.observer.ObserveSourceQueueDequeued(residence)
			}
			if session.queueObserver != nil {
				session.queueObserver.ObserveSourceQueueDequeued(residence)
			}
			if !session.current(item.generation) {
				if session.observer != nil {
					session.observer.ObserveSourceQueueDiscardedStale()
				}
				if session.queueObserver != nil {
					session.queueObserver.ObserveSourceQueueDiscardedStale()
				}
				continue
			}
			if item.action.Kind == control.InputPointerMove && item.action.DeltaX == 0 && item.action.DeltaY == 0 {
				continue
			}
			if err := session.sink.SendAction(handoff.Offered(ctx, item.queuedAt), item.action, func() bool { return session.current(item.generation) }); err != nil {
				if !session.current(item.generation) && sourceErrorsOnly(err, control.ErrSourceActionDiscarded) {
					if session.observer != nil {
						session.observer.ObserveSourceQueueDiscardedStale()
					}
					if session.queueObserver != nil {
						session.queueObserver.ObserveSourceQueueDiscardedStale()
					}
					continue
				}
				return err
			}
		}
	}
}

func (session *SourceSession) stopSender(
	ctx context.Context,
	cancelSend context.CancelFunc,
	senderDone <-chan error,
	cause error,
	fail bool,
) error {
	return session.cleanupSession(ctx, cancelSend, senderDone, cause, fail)
}

func (session *SourceSession) abort(cause error) {
	// Retire synchronously: a late activation must not race the worker's
	// asynchronous fault handling and resume suppression for this owner.
	session.terminal.Store(true)
	// Active, Offer, current and Activate all honor terminal. Ordinary worker
	// cleanup owns deactivation; a hook must not wait for that worker's mutex.
	if session.faultSent.CompareAndSwap(false, true) {
		select {
		case session.faults <- cause:
		default:
		}
	}
}

func (session *SourceSession) current(generation uint64) bool {
	if session.terminal.Load() {
		return false
	}
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	return !session.terminal.Load() && session.active && session.generation == generation
}

func (session *SourceSession) cleanupSession(
	ctx context.Context,
	cancelSend context.CancelFunc,
	senderDone <-chan error,
	cause error,
	fail bool,
) error {
	session.Deactivate()
	state := uint32(1) // Cleanup entered; each join is observed independently.
	if senderDone == nil {
		state |= 2 // Caller already consumed the sender's result.
	}
	session.cleanupState.Store(state)
	timeout := session.cleanup
	if timeout <= 0 {
		timeout = SourceCleanupTimeout
	}
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeout)
	defer cancel()
	interruptDone := make(chan error, 1)
	go func() { interruptDone <- session.sink.Interrupt(cause) }()
	if cancelSend != nil {
		cancelSend()
	}

	var interruptErr error
	var senderErr error
	for interruptDone != nil || senderDone != nil {
		select {
		case interruptErr = <-interruptDone:
			interruptDone = nil
			session.cleanupState.Or(4)
		case senderErr = <-senderDone:
			senderDone = nil
			session.cleanupState.Or(2)
			if sourceErrorsOnly(senderErr, context.Canceled, ctx.Err()) {
				senderErr = nil
			}
		case <-cleanupCtx.Done():
			return errors.Join(cause, interruptErr, senderErr, ErrSourceCleanupIncomplete, cleanupCtx.Err())
		}
	}
	if !fail {
		return errors.Join(cause, interruptErr, senderErr)
	}
	failDone := make(chan error, 1)
	go func() { failDone <- session.sink.Fail(cleanupCtx, cause) }()
	select {
	case failErr := <-failDone:
		return errors.Join(cause, interruptErr, senderErr, failErr)
	case <-cleanupCtx.Done():
		return errors.Join(cause, interruptErr, senderErr, ErrSourceCleanupIncomplete, cleanupCtx.Err())
	}
}
