package agent

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestTerminationKeepsTriggerSeparateFromLateNativeCleanup(t *testing.T) {
	left, right := openConnectionPair(t)
	defer left.Close()
	defer right.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	hook := &lateCleanupHook{ready: make(chan struct{}), failure: errors.Join(wininput.ErrSourceCaptureUnregisterFailed, syscall.Errno(170))}
	config := DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10})
	var observed []SessionTermination
	config.TerminationObserver = func(v SessionTermination) { observed = append(observed, v) }
	done := make(chan error, 1)
	go func() {
		_, err := RunSession(ctx, config, Dependencies{Connection: right, Hook: hook, Input: newTestInput(false), Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}})
		done <- err
	}()
	<-hook.ready
	cancel()
	err := <-done
	if !errors.Is(err, ErrRecoveryUnverified) || !errors.Is(err, syscall.Errno(170)) {
		t.Fatalf("lost cleanup failure: %v", err)
	}
	if len(observed) < 2 || !observed[0].Trigger || !observed[0].Canceled || observed[0].NativeKnown {
		t.Fatalf("late error contaminated trigger: %+v", observed)
	}
	found := false
	for _, v := range observed[1:] {
		if v.Trigger || v.ObservedAt.Before(observed[0].ObservedAt) {
			t.Fatalf("invalid termination order: %+v", observed)
		}
		if v.Component == "source-hook" {
			found = true
			if !v.NativeKnown || v.NativeError != 170 || v.Failure.Code != "unregister_failed" {
				t.Fatalf("native detail lost: %+v", v)
			}
		}
	}
	if !found {
		t.Fatal("source owner result missing")
	}
}
