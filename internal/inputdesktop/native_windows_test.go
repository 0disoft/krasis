//go:build windows

package inputdesktop

import (
	"context"
	"errors"
	"runtime"
	"testing"
)

func TestWindowsOwnedThreadSurvivesSchedulerYield(t *testing.T) {
	ctx := context.Background()
	desktop := &testBinding{}
	err := run(ctx, func(scope *Scope) error {
		owner := currentThreadID()
		for range 100 {
			runtime.Gosched()
			if currentThreadID() != owner {
				t.Error("owned OS thread changed")
			}
		}
		result := make(chan error, 1)
		go func() {
			result <- scope.Do(ctx, func() error { t.Error("foreign goroutine executed work"); return nil })
		}()
		if err := <-result; !errors.Is(err, ErrWrongThread) {
			t.Errorf("foreign goroutine result = %v", err)
		}
		return scope.Do(ctx, func() error { return nil })
	}, func() (binding, error) { return desktop, nil }, currentThreadID, runtime.LockOSThread, runtime.UnlockOSThread)
	if err != nil || desktop.closed.Load() != 1 {
		t.Fatalf("owned thread lifetime: %v", err)
	}
}

func TestWindowsNativeBindingWithoutInput(t *testing.T) {
	called := false
	err := Run(context.Background(), func(scope *Scope) error {
		return scope.Do(context.Background(), func() error { called = true; return nil })
	})
	if errors.Is(err, ErrUnavailable) && !errors.Is(err, ErrCleanup) {
		t.Skip("no accessible active Default desktop in this test environment")
	}
	if err != nil || !called {
		t.Fatalf("native desktop binding without input: %v", err)
	}
	if AccessMask != 171 || AccessMask&0x100 != 0 {
		t.Fatal("desktop access mask changed from the g104 acceptance")
	}
}
