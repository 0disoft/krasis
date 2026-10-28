//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestCaptureCursorPresentationDoesNotQueryDesktopAuthority(t *testing.T) {
	old := callSetSourceCaptureCursor
	t.Cleanup(func() { callSetSourceCaptureCursor = old })
	calls := 0
	callSetSourceCaptureCursor = func(args ...uintptr) (uintptr, uintptr, error) {
		if len(args) != 1 || args[0] != 0 {
			t.Fatal("cursor presentation changed a resource other than the cursor shape")
		}
		calls++
		return 0, 0, nil
	}
	// A scope that cannot authorize input makes accidental Scope.Do calls
	// observable. Only the already-activated owner's cosmetic path may proceed.
	ctx := context.WithValue(context.Background(), sourceLoginDesktopKey{}, &inputdesktop.Scope{})
	current := &sourceWheelCaptureRun{loginContext: ctx}
	if current.hideActiveCursor() || calls != 0 {
		t.Fatal("inactive owner hid the cursor")
	}
	current.active.Store(true)
	for range 3 {
		if !current.hideActiveCursor() {
			t.Fatal("cosmetic cursor callback entered desktop authorization")
		}
	}
	current.active.Store(false)
	if current.hideActiveCursor() || calls != 3 {
		t.Fatal("return cleanup retained cursor suppression")
	}
	// Removing authority I/O from painting must not make activation possible.
	if err := current.activateWindow(); !errors.Is(err, inputdesktop.ErrWrongThread) || current.active.Load() {
		t.Fatal("cursor presentation bypassed activation authority", err)
	}
}

func TestCaptureCursorPresentationStopsOnCancellationAndFailure(t *testing.T) {
	old := callSetSourceCaptureCursor
	t.Cleanup(func() { callSetSourceCaptureCursor = old })
	callSetSourceCaptureCursor = func(...uintptr) (uintptr, uintptr, error) {
		t.Fatal("retired capture hid the cursor")
		return 0, 0, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	current := &sourceWheelCaptureRun{loginContext: ctx}
	current.active.Store(true)
	cancel()
	if current.hideActiveCursor() {
		t.Fatal("cancelled owner handled cursor presentation")
	}
	current.loginContext = context.Background()
	current.recordError(errors.New("synthetic native failure"), false)
	if current.hideActiveCursor() {
		t.Fatal("failed owner handled cursor presentation")
	}
}
