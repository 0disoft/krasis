// Package inputdesktop owns a fixed desktop handle on one dedicated OS thread.
// It provides no privilege elevation, input injection, or authorization endpoint.
package inputdesktop

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
)

var (
	ErrUnavailable   = errors.New("input desktop is unavailable")
	ErrInactive      = errors.New("owned desktop is not the selected active input desktop")
	ErrWrongThread   = errors.New("input desktop operation is on another OS thread")
	ErrClosed        = errors.New("input desktop scope is closed")
	ErrInvalid       = errors.New("input desktop context and operation are required")
	ErrCleanup       = errors.New("input desktop cleanup failed; worker process must exit")
	ErrCallbackPanic = errors.New("input desktop callback panicked")
	ErrCallbackExit  = errors.New("input desktop callback exited without returning")
)

// AccessMask preserves the handle rights accepted in the attended g104 trial.
// It includes DESKTOP_JOURNALPLAYBACK, but not desktop-switch or ACL rights.
const AccessMask uint32 = 0xAB

type binding interface {
	active() error
	close() error
}

// Scope may only execute operations on the thread owned by Run. An input
// caller must independently authorize the exact peer, lease, epoch and sequence.
type Scope struct {
	root    context.Context
	desktop binding
	owner   uintptr
	thread  func() uintptr
	closed  atomic.Bool
}

// Do checks ownership and desktop activity immediately before an operation.
// Cancellation after operation begins does not prove that input was unapplied.
// The operation must not change desktops, retain native handles or start work
// on another goroutine. It must return its original injection outcome unchanged.
func (scope *Scope) Do(ctx context.Context, operation func() error) error {
	if ctx == nil || operation == nil {
		return ErrInvalid
	}
	if scope == nil || scope.closed.Load() {
		return ErrClosed
	}
	if scope.owner == 0 || scope.thread() != scope.owner {
		return ErrWrongThread
	}
	if err := scope.root.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	var activeErr error
	if interactive, ok := scope.desktop.(interface {
		activeContext(context.Context, context.Context) error
	}); ok {
		activeErr = interactive.activeContext(ctx, scope.root)
	} else {
		activeErr = scope.desktop.active()
	}
	if activeErr != nil {
		return errors.Join(ErrInactive, activeErr)
	}
	// Desktop queries can yield. Recheck cancellation before invoking the caller.
	if err := scope.root.Err(); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return operation()
}

// Run invokes a bounded, cooperative callback on a dedicated locked OS thread.
// It waits for callback completion and cleanup even if ctx is canceled. On
// ErrCleanup the future privileged worker must exit; uncertain input cannot be
// replayed on another backend. A damaged OS thread is never returned to Go's pool.
func Run(ctx context.Context, callback func(*Scope) error) error {
	return run(ctx, callback, openDesktop, currentThreadID, runtime.LockOSThread, runtime.UnlockOSThread)
}

func run(ctx context.Context, callback func(*Scope) error, open func() (binding, error),
	thread func() uintptr, lock, unlock func()) error {
	if ctx == nil || callback == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	result := make(chan error, 1)
	go func() {
		lock()
		if err := ctx.Err(); err != nil {
			unlock()
			result <- err
			return
		}
		desktop, err := open()
		if err != nil {
			if !errors.Is(err, ErrCleanup) {
				unlock()
			}
			result <- err
			return
		}
		scope := &Scope{root: ctx, desktop: desktop, owner: thread(), thread: thread}
		// Goexit runs defers but does not return from invoke. Cleanup and result
		// publication must therefore live in a defer on the owning goroutine.
		err = ErrCallbackExit
		defer func() {
			scope.closed.Store(true)
			cleanupErr := desktop.close()
			if cleanupErr == nil {
				unlock()
			} else {
				err = errors.Join(err, ErrCleanup, cleanupErr)
			}
			// Go discards the locked OS thread on cleanup failure.
			result <- err
		}()
		if scope.owner == 0 {
			err = ErrWrongThread
		} else {
			err = scope.Do(ctx, func() error { return invoke(callback, scope) })
		}
	}()
	return <-result
}

func invoke(callback func(*Scope) error, scope *Scope) (err error) {
	defer func() {
		if recover() != nil {
			// A panic value could include input or user data. Do not echo it.
			err = ErrCallbackPanic
		}
	}()
	return callback(scope)
}
