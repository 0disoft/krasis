//go:build windows

package wininput

import (
	"context"
	"errors"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/application"
)

func TestNativeCursorObservationFailureRetainsNativeCauseAndReadOnlyMarker(t *testing.T) {
	previous := callGetCursorPos
	t.Cleanup(func() { callGetCursorPos = previous })
	calls := 0
	callGetCursorPos = func(*point) (uintptr, uintptr, error) { calls++; return 0, 0, syscall.Errno(5) }
	_, _, err := (nativeInjectionPort{}).CursorPosition(context.Background())
	if calls != 1 || !errors.Is(err, syscall.Errno(5)) || !errors.Is(err, application.ErrCursorObservationUnavailable) || !errors.Is(err, ErrInputInjection) {
		t.Fatal("native observation evidence lost", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, _, err = (nativeInjectionPort{}).CursorPosition(ctx)
	if calls != 1 || errors.Is(err, application.ErrCursorObservationUnavailable) {
		t.Fatal("cancelled read became a retryable native observation", err)
	}
}
