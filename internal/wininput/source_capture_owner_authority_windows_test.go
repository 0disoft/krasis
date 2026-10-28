//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestProtectedCaptureRetainsPostProofAndCustomGeometryGuards(t *testing.T) {
	hook, err := NewProtectedSourceHook(func() error { return nil })
	if err != nil {
		t.Fatal(err)
	}
	post, ok := hook.SourceCapture().(*guardedSourceActivation)
	if !ok || post.beforeAuthorized == nil || post.beforeAuthorized() || post.SourceActivation != hook.capture || post.check == nil {
		t.Fatal("protected capture lacks its post-activation proof")
	}
	hook.SetBoundaryGuard(func() error { return ErrDisplayGeometryChanged })
	geometry := hook.SourceCapture().(*guardedSourceActivation)
	if geometry.beforeAuthorized != nil || geometry.check() != ErrDisplayGeometryChanged {
		t.Fatal("custom geometry guard lost its pre-activation check")
	}
	inner, ok := geometry.SourceActivation.(*guardedSourceActivation)
	if !ok || inner.beforeAuthorized == nil {
		t.Fatal("geometry guard displaced the mandatory post-proof")
	}
	ordinary := NewSourceHook()
	ordinary.SetBoundaryGuard(func() error { return nil })
	if ordinary.SourceCapture().(*guardedSourceActivation).beforeAuthorized != nil {
		t.Fatal("ordinary capture assumed a privileged native owner")
	}
}

func TestCaptureCommandProofIsSingleUseThreadBoundAndNeverRetained(t *testing.T) {
	previousThread := sourceWheelCaptureCurrentThread
	defer func() { sourceWheelCaptureCurrentThread = previousThread }()
	thread := uint32(44)
	sourceWheelCaptureCurrentThread = func() uint32 { return thread }
	checks := 0
	hook, _ := NewProtectedSourceHook(func() error { checks++; return nil })
	capture := hook.SourceCapture().(*guardedSourceActivation)
	native := &geometryCapture{}
	capture.SourceActivation = native
	activate := func() error { err := capture.Activate(); native.Deactivate(); return err }
	if err := hook.sourceCommandWork(context.Background(), func() error {
		if err := activate(); err != nil || checks != 1 {
			t.Fatal("fresh command repeated its pre-proof", err, checks)
		}
		if err := activate(); err != nil || checks != 3 {
			t.Fatal("command proof was reused twice", err, checks)
		}
		return nil
	})(); err != nil {
		t.Fatal(err)
	}
	if hook.commandProof.Load() != nil {
		t.Fatal("command retained authority after returning")
	}
	if err := activate(); err != nil || checks != 5 {
		t.Fatal("retained activation skipped a fresh pre-proof", err, checks)
	}
	for _, mode := range []string{"wrong-thread", "cancelled"} {
		ctx, cancel := context.WithCancel(context.Background())
		if err := hook.sourceCommandWork(ctx, func() error {
			if mode == "wrong-thread" {
				thread++
			} else {
				cancel()
			}
			if capture.beforeAuthorized() {
				t.Fatal("invalid command proof admitted capture", mode)
			}
			return nil
		})(); err != nil {
			t.Fatal(err)
		}
		cancel()
		if hook.commandProof.Load() != nil {
			t.Fatal("invalid scope retained its proof")
		}
	}
}

func TestNativeCaptureRejectsInvalidOwnerScopeBeforeWindowActivation(t *testing.T) {
	restore := stubSourceWheelCaptureActivation(t)
	defer restore()
	sourceWheelCaptureSetForeground = func(uintptr) error { t.Fatal("invalid desktop proof changed foreground"); return nil }
	sourceWheelCaptureSetPosition = func(uintptr, int32, int32) error { t.Fatal("invalid desktop proof moved capture"); return nil }
	owner := &sourceWheelCaptureRun{window: 10, loginContext: context.WithValue(context.Background(), sourceLoginDesktopKey{}, &inputdesktop.Scope{})}
	if err := owner.activateWindowProfile(context.Background()); !errors.Is(err, inputdesktop.ErrWrongThread) || owner.active.Load() {
		t.Fatal("native owner did not enforce the fresh pre-activation scope", err)
	}
}
