package inputdesktop

import (
	"context"
	"errors"
)

type retiringBinding interface {
	binding
	closeRetired() error
}

type retiringThread interface {
	join() error
	close() error
}

// Capture threads are disposable native owners. Their Go goroutine exits while
// locked; an exact retained kernel thread handle proves exit before HDESK close.
// No window/hook-bearing thread is restored or returned to the runtime pool.
func runRetiring(ctx context.Context, callback func(*Scope) error,
	open func() (retiringBinding, error), thread func() uintptr,
	pin func() (retiringThread, error), startLocked func(func())) error {
	if ctx == nil || callback == nil {
		return ErrInvalid
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	type outcome struct {
		desktop retiringBinding
		owner   retiringThread
		err     error
	}
	done := make(chan outcome, 1)
	startLocked(func() {
		// The launcher owns the lock. No UnlockOSThread, including failures.
		result := outcome{err: ErrCallbackExit}
		var scope *Scope
		defer func() {
			if scope != nil {
				scope.closed.Store(true)
			}
			done <- result
		}()
		if err := ctx.Err(); err != nil {
			result.err = err
			return
		}
		var err error
		result.owner, err = pin()
		if err != nil {
			result.err = err
			return
		}
		result.desktop, err = open()
		if err != nil {
			result.err = err
			return
		}
		scope = &Scope{root: ctx, desktop: result.desktop, owner: thread(), thread: thread}
		if scope.owner == 0 {
			result.err = ErrWrongThread
			return
		}
		result.err = scope.Do(ctx, func() error { return invoke(callback, scope) })
	})
	result := <-done
	if result.owner == nil {
		return result.err // No desktop acquisition could start without a thread pin.
	}
	if err := result.owner.join(); err != nil {
		// The desktop may still be attached: retain it until process retirement.
		result.err = errors.Join(result.err, ErrCleanup, operationFailure("desktop-thread-exit", err))
	} else if result.desktop != nil {
		if err := result.desktop.closeRetired(); err != nil {
			result.err = errors.Join(result.err, ErrCleanup, err)
		}
	}
	if err := result.owner.close(); err != nil {
		result.err = errors.Join(result.err, ErrCleanup, operationFailure("desktop-thread-exit", err))
	}
	return result.err
}
