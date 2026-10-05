//go:build windows

package wininput

import (
	"context"
	"unsafe"

	"github.com/0disoft/krasis/internal/application"
)

type cursorBounds struct{ Left, Top, Right, Bottom int32 }

var procGetClipCursor = user32.NewProc("GetClipCursor")
var readCursorBounds = func(bounds *cursorBounds) bool {
	ok, _, _ := procGetClipCursor.Call(uintptr(unsafe.Pointer(bounds)))
	return ok != 0
}

var _ application.CursorPlacementPort = nativeInjectionPort{}

// The installed worker calls this on its authorized desktop-owning thread.
// Observe confinement, set once and read back without another pipe operation.
// A changed constraint or physical movement still fails the caller's comparison.
func (port nativeInjectionPort) PlaceCursor(ctx context.Context, x, y int32) (application.CursorPlacement, error) {
	var receipt application.CursorPlacement
	if ctx == nil || ctx.Err() != nil {
		return receipt, &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: ErrInputInjection}
	}
	var clip cursorBounds
	if !readCursorBounds(&clip) {
		return receipt, &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: ErrInputInjection}
	}
	target, err := confinedCursorTarget(x, y, clip, systemMetric(smXVIRTUALSCREEN), systemMetric(smYVIRTUALSCREEN), systemMetric(smCXVIRTUALSCREEN), systemMetric(smCYVIRTUALSCREEN))
	if err != nil {
		return receipt, &application.InjectionError{Certainty: application.InjectionNotApplied, Cause: err}
	}
	if err := port.SetCursorPosition(ctx, target.X, target.Y); err != nil {
		return receipt, err
	}
	actualX, actualY, err := port.CursorPosition(ctx)
	if err != nil {
		return receipt, &application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: err}
	}
	return application.CursorPlacement{TargetX: target.X, TargetY: target.Y, X: actualX, Y: actualY}, nil
}

// RECT's right/bottom bounds are exclusive. Intersect with current screen
// geometry; malformed or disjoint confinement must not cause a cursor write.
func confinedCursorTarget(x, y int32, clip cursorBounds, left, top, width, height int32) (point, error) {
	l, t := int64(left), int64(top)
	r, b := l+int64(width), t+int64(height)
	if width <= 0 || height <= 0 || int64(x) < l || int64(x) >= r || int64(y) < t || int64(y) >= b || clip.Left >= clip.Right || clip.Top >= clip.Bottom {
		return point{}, ErrUnsupportedInput
	}
	l, t = max(l, int64(clip.Left)), max(t, int64(clip.Top))
	r, b = min(r, int64(clip.Right)), min(b, int64(clip.Bottom))
	if l >= r || t >= b {
		return point{}, ErrUnsupportedInput
	}
	return point{X: int32(max(l, min(int64(x), r-1))), Y: int32(max(t, min(int64(y), b-1)))}, nil
}
