package wininput

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

const MaxSourceQueueCapacity = 65536

const SourceCleanupTimeout = 2 * time.Second

var (
	ErrSourceQueueOverflow = errors.New("source input queue overflow")
	ErrSourceSession       = errors.New("source input session failed")
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
	sink       SourceSink
	observer   SourcePipelineObserver
	cleanup    time.Duration
	queue      chan queuedAction
	queueDepth atomic.Int64
	faults     chan error
	now        func() time.Time
	started    atomic.Bool
	terminal   atomic.Bool
	stateMu    sync.RWMutex
	active     bool
	generation uint64
	faultOnce  sync.Once
}

func NewSourceSession(capacity int, sink SourceSink) (*SourceSession, error) {
	return NewSourceSessionObserved(capacity, sink, nil)
}

func NewSourceSessionObserved(capacity int, sink SourceSink, observer SourcePipelineObserver) (*SourceSession, error) {
	if capacity <= 0 || capacity > MaxSourceQueueCapacity {
		return nil, fmt.Errorf("source queue capacity must be between 1 and %d", MaxSourceQueueCapacity)
	}
	if sink == nil {
		return nil, errors.New("source sink is required")
	}
	return &SourceSession{
		sink:     sink,
		observer: observer,
		cleanup:  SourceCleanupTimeout,
		queue:    make(chan queuedAction, capacity),
		faults:   make(chan error, 1),
		now:      time.Now,
	}, nil
}

func (session *SourceSession) Activate() error {
	if session == nil || !session.started.Load() || session.terminal.Load() {
		return errors.New("source session worker is not running")
	}
	session.stateMu.Lock()
	defer session.stateMu.Unlock()
	if session.active {
		return errors.New("source session is already active")
	}
	session.generation++
	session.active = true
	return nil
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
	if session == nil {
		return false
	}
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	return session.active
}

// Offer is safe for a low-level hook callback. It never blocks. It returns true
// only when the physical event was accepted for the current outbound generation
// and may therefore be suppressed locally.
func (session *SourceSession) Offer(action control.InputAction) bool {
	if session == nil {
		return false
	}
	session.stateMu.RLock()
	if !session.active {
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
	depth := session.queueDepth.Add(1)
	select {
	case session.queue <- queuedAction{generation: generation, action: action, queuedAt: queuedAt}:
		session.stateMu.RUnlock()
		if session.observer != nil {
			session.observer.ObserveSourceQueueEnqueued(uint64(depth))
		}
		session.observeWheelQueue(action, SourceQueueAccepted)
		return true
	default:
		session.queueDepth.Add(-1)
		session.stateMu.RUnlock()
		session.abort(ErrSourceQueueOverflow)
		session.observeWheelQueue(action, SourceQueueRejectedOverflow)
		return false
	}
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
		case item := <-session.queue:
			session.queueDepth.Add(-1)
			residence := session.now().Sub(item.queuedAt)
			if residence < 0 {
				residence = 0
			}
			if session.observer != nil {
				session.observer.ObserveSourceQueueDequeued(residence)
			}
			if !session.current(item.generation) {
				if session.observer != nil {
					session.observer.ObserveSourceQueueDiscardedStale()
				}
				continue
			}
			if err := session.sink.SendAction(ctx, item.action, func() bool { return session.current(item.generation) }); err != nil {
				if !session.current(item.generation) {
					if session.observer != nil {
						session.observer.ObserveSourceQueueDiscardedStale()
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
	session.stateMu.Lock()
	session.active = false
	session.generation++
	session.stateMu.Unlock()
	session.faultOnce.Do(func() {
		select {
		case session.faults <- cause:
		default:
		}
	})
}

func (session *SourceSession) current(generation uint64) bool {
	session.stateMu.RLock()
	defer session.stateMu.RUnlock()
	return session.active && session.generation == generation
}

func (session *SourceSession) cleanupSession(
	ctx context.Context,
	cancelSend context.CancelFunc,
	senderDone <-chan error,
	cause error,
	fail bool,
) error {
	session.Deactivate()
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
		case senderErr = <-senderDone:
			senderDone = nil
			if errors.Is(senderErr, context.Canceled) || (ctx.Err() != nil && errors.Is(senderErr, ctx.Err())) {
				senderErr = nil
			}
		case <-cleanupCtx.Done():
			return errors.Join(cause, interruptErr, senderErr, cleanupCtx.Err())
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
		return errors.Join(cause, interruptErr, senderErr, cleanupCtx.Err())
	}
}
