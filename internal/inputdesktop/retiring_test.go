package inputdesktop

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"syscall"
	"testing"
	"time"
)

type testRetiringBinding struct {
	testBinding
	retired   atomic.Int32
	retireErr error
}

func (b *testRetiringBinding) closeRetired() error { b.retired.Add(1); return b.retireErr }

type testRetiringThread struct {
	waiting, release  chan struct{}
	joinErr, closeErr error
	closed            atomic.Int32
}

func (t *testRetiringThread) join() error {
	if t.waiting != nil {
		close(t.waiting)
		<-t.release
	}
	return t.joinErr
}
func (t *testRetiringThread) close() error { t.closed.Add(1); return t.closeErr }

func testRetiringRun(ctx context.Context, callback func(*Scope) error, desktop *testRetiringBinding, owner *testRetiringThread) error {
	return runRetiring(ctx, callback, func() (retiringBinding, error) { return desktop, nil },
		func() uintptr { return 1 }, func() (retiringThread, error) { return owner, nil }, func() {})
}

func TestCaptureRetirementWaitsForNativeExitBeforeClosingDesktop(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	desktop := &testRetiringBinding{}
	owner := &testRetiringThread{waiting: make(chan struct{}), release: make(chan struct{})}
	done := make(chan error, 1)
	var scope *Scope
	go func() { done <- testRetiringRun(ctx, func(s *Scope) error { scope = s; return nil }, desktop, owner) }()
	select {
	case <-owner.waiting:
	case <-time.After(time.Second):
		t.Fatal("thread exit wait missing")
	}
	cancel() // Cancellation cannot substitute for kernel thread exit proof.
	if desktop.retired.Load() != 0 || desktop.closed.Load() != 0 || owner.closed.Load() != 0 {
		t.Fatal("desktop restored or closed while native thread was still live")
	}
	if err := scope.Do(context.Background(), func() error { t.Error("retired scope performed work"); return nil }); !errors.Is(err, ErrClosed) {
		t.Fatal("scope remained usable while waiting for native exit", err)
	}
	select {
	case <-done:
		t.Fatal("retirement returned before native exit")
	default:
	}
	close(owner.release)
	if err := <-done; err != nil || desktop.retired.Load() != 1 || desktop.closed.Load() != 0 || owner.closed.Load() != 1 {
		t.Fatal("clean thread retirement lost", err)
	}
}

func TestCaptureRetirementCannotHideNativeCleanupOrUnconfirmedExit(t *testing.T) {
	for _, failedAt := range []string{"exit", "desktop", "thread handle"} {
		t.Run(failedAt, func(t *testing.T) {
			desktop, owner := &testRetiringBinding{}, &testRetiringThread{}
			failure := syscall.Errno(170)
			switch failedAt {
			case "exit":
				owner.joinErr = failure
			case "desktop":
				desktop.retireErr = operationFailure("desktop-close", failure)
			case "thread handle":
				owner.closeErr = failure
			}
			primary := errors.New("owned window cleanup failed")
			err := testRetiringRun(context.Background(), func(*Scope) error { return primary }, desktop, owner)
			if !errors.Is(err, primary) || !errors.Is(err, failure) || !errors.Is(err, ErrCleanup) || owner.closed.Load() != 1 || desktop.closed.Load() != 0 {
				t.Fatal("cleanup uncertainty or primary cause lost", err)
			}
			if failedAt == "exit" && desktop.retired.Load() != 0 {
				t.Fatal("attached desktop closed without thread exit")
			}
		})
	}
}

func TestCaptureRetirementGoexitAndPanicStillJoinNativeOwner(t *testing.T) {
	for _, exit := range []bool{false, true} {
		desktop, owner := &testRetiringBinding{}, &testRetiringThread{}
		err := testRetiringRun(context.Background(), func(*Scope) error {
			if exit {
				runtime.Goexit()
			}
			panic("PRIVATE/input/value")
		}, desktop, owner)
		want := ErrCallbackPanic
		if exit {
			want = ErrCallbackExit
		}
		if !errors.Is(err, want) || desktop.retired.Load() != 1 || owner.closed.Load() != 1 {
			t.Fatal("callback abnormal exit skipped native retirement", err)
		}
	}
}
