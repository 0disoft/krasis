package inputdesktop

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
)

type testBinding struct {
	activeErr error
	closeErr  error
	activeFn  func()
	closed    atomic.Int32
}

func (desktop *testBinding) active() error {
	if desktop.activeFn != nil {
		desktop.activeFn()
	}
	return desktop.activeErr
}
func (desktop *testBinding) close() error {
	desktop.closed.Add(1)
	return desktop.closeErr
}

func testRun(ctx context.Context, callback func(*Scope) error, desktop *testBinding,
	thread func() uintptr, unlocked *atomic.Int32) error {
	return run(ctx, callback, func() (binding, error) { return desktop, nil }, thread,
		func() {}, func() { unlocked.Add(1) })
}

func TestScopeOwnershipAndClosedLifetime(t *testing.T) {
	ctx := context.Background()
	desktop := &testBinding{}
	var threadID atomic.Uintptr
	threadID.Store(1)
	var unlocked atomic.Int32
	var retained *Scope
	called := 0
	err := testRun(ctx, func(scope *Scope) error {
		retained = scope
		if err := scope.Do(ctx, func() error { called++; return nil }); err != nil {
			return err
		}
		threadID.Store(2)
		if err := scope.Do(ctx, func() error { called++; return nil }); !errors.Is(err, ErrWrongThread) {
			t.Errorf("wrong-thread operation = %v", err)
		}
		return nil
	}, desktop, threadID.Load, &unlocked)
	if err != nil || called != 1 || desktop.closed.Load() != 1 || unlocked.Load() != 1 {
		t.Fatalf("scope lifetime: err=%v calls=%d closes=%d unlocks=%d", err, called, desktop.closed.Load(), unlocked.Load())
	}
	if err := retained.Do(ctx, func() error { called++; return nil }); !errors.Is(err, ErrClosed) || called != 1 {
		t.Fatalf("retired scope performed an operation: %v", err)
	}
}

func TestCancellationWhileCheckingDesktopDoesNotInvokeOperation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	checks := 0
	desktop := &testBinding{activeFn: func() {
		checks++
		if checks == 2 {
			cancel()
		}
	}}
	var unlocked atomic.Int32
	called := false
	err := testRun(ctx, func(scope *Scope) error {
		return scope.Do(ctx, func() error { called = true; return nil })
	}, desktop, func() uintptr { return 1 }, &unlocked)
	if !errors.Is(err, context.Canceled) || called || desktop.closed.Load() != 1 {
		t.Fatalf("cancellation escaped input boundary: %v called=%v", err, called)
	}
}

func TestInactiveDesktopRefusesCallbackAndCleansUp(t *testing.T) {
	desktop := &testBinding{activeErr: ErrInactive}
	var unlocked atomic.Int32
	called := false
	err := testRun(context.Background(), func(*Scope) error { called = true; return nil },
		desktop, func() uintptr { return 1 }, &unlocked)
	if !errors.Is(err, ErrInactive) || called || desktop.closed.Load() != 1 || unlocked.Load() != 1 {
		t.Fatalf("inactive desktop ownership failed: %v", err)
	}
}

func TestPanicDoesNotEchoDataAndStillCleansUp(t *testing.T) {
	desktop := &testBinding{}
	var unlocked atomic.Int32
	err := testRun(context.Background(), func(*Scope) error { panic("synthetic-private-payload") },
		desktop, func() uintptr { return 1 }, &unlocked)
	if err != ErrCallbackPanic || desktop.closed.Load() != 1 || unlocked.Load() != 1 {
		t.Fatalf("panic cleanup: %v", err)
	}
}

func TestCleanupFailureDoesNotReturnThreadToPool(t *testing.T) {
	bodyErr := errors.New("operation outcome unknown")
	desktop := &testBinding{closeErr: errors.New("restore refused")}
	var unlocked atomic.Int32
	err := testRun(context.Background(), func(*Scope) error { return bodyErr },
		desktop, func() uintptr { return 1 }, &unlocked)
	if !errors.Is(err, bodyErr) || !errors.Is(err, ErrCleanup) || desktop.closed.Load() != 1 || unlocked.Load() != 0 {
		t.Fatalf("unsafe cleanup outcome: %v unlocks=%d", err, unlocked.Load())
	}
}

func TestCallbackGoexitStillClosesAndReturns(t *testing.T) {
	desktop := &testBinding{}
	var unlocked atomic.Int32
	err := testRun(context.Background(), func(*Scope) error {
		runtime.Goexit()
		return nil
	}, desktop, func() uintptr { return 1 }, &unlocked)
	if !errors.Is(err, ErrCallbackExit) || desktop.closed.Load() != 1 || unlocked.Load() != 1 {
		t.Fatalf("callback exit cleanup: %v closes=%d unlocks=%d", err, desktop.closed.Load(), unlocked.Load())
	}
}

func TestRejectedRunNeverOpensDesktop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	opened := false
	err := run(ctx, func(*Scope) error { return nil }, func() (binding, error) {
		opened = true
		return &testBinding{}, nil
	}, func() uintptr { return 1 }, func() {}, func() {})
	if !errors.Is(err, context.Canceled) || opened {
		t.Fatalf("canceled run touched native desktop: %v", err)
	}
	if err := Run(nil, nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid callback = %v", err)
	}
}

func TestOpenCleanupFailureDoesNotUnlock(t *testing.T) {
	var unlocked atomic.Int32
	err := run(context.Background(), func(*Scope) error { t.Error("unexpected callback"); return nil },
		func() (binding, error) { return nil, errors.Join(ErrUnavailable, ErrCleanup) },
		func() uintptr { return 1 }, func() {}, func() { unlocked.Add(1) })
	if !errors.Is(err, ErrCleanup) || unlocked.Load() != 0 {
		t.Fatalf("open cleanup failure released thread: %v", err)
	}
}
