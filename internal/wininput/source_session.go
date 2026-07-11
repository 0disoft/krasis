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
	SendAction(context.Context, control.InputAction) error
	Fail(context.Context, error) error
}

type queuedAction struct {
	generation uint64
	action     control.InputAction
}

type SourceSession struct {
	sink       SourceSink
	queue      chan queuedAction
	faults     chan error
	started    atomic.Bool
	terminal   atomic.Bool
	stateMu    sync.RWMutex
	active     bool
	generation uint64
	faultOnce  sync.Once
}

func NewSourceSession(capacity int, sink SourceSink) (*SourceSession, error) {
	if capacity <= 0 || capacity > MaxSourceQueueCapacity {
		return nil, fmt.Errorf("source queue capacity must be between 1 and %d", MaxSourceQueueCapacity)
	}
	if sink == nil {
		return nil, errors.New("source sink is required")
	}
	return &SourceSession{
		sink:   sink,
		queue:  make(chan queuedAction, capacity),
		faults: make(chan error, 1),
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
		return false
	}
	if err := action.Validate(); err != nil {
		session.stateMu.RUnlock()
		session.abort(errors.Join(ErrSourceSession, err))
		return false
	}
	generation := session.generation
	select {
	case session.queue <- queuedAction{generation: generation, action: action}:
		session.stateMu.RUnlock()
		return true
	default:
		session.stateMu.RUnlock()
		session.abort(ErrSourceQueueOverflow)
		return false
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
	for {
		select {
		case <-ctx.Done():
			if session.Active() {
				session.Deactivate()
				return session.failSink(ctx, ctx.Err())
			}
			return ctx.Err()
		case cause := <-session.faults:
			return session.failSink(ctx, cause)
		case item := <-session.queue:
			if !session.current(item.generation) {
				continue
			}
			if err := session.sink.SendAction(ctx, item.action); err != nil {
				session.abort(err)
			}
		}
	}
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

func (session *SourceSession) failSink(ctx context.Context, cause error) error {
	cleanupCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), SourceCleanupTimeout)
	defer cancel()
	if err := session.sink.Fail(cleanupCtx, cause); err != nil {
		return errors.Join(cause, err)
	}
	return cause
}
