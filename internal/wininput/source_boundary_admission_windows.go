//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
)

const (
	guiInMoveSize     = 0x00000002
	guiInMenuMode     = 0x00000004
	guiSystemMenuMode = 0x00000008
	guiPopupMenuMode  = 0x00000010
	guiMenuModeMask   = guiInMenuMode | guiSystemMenuMode | guiPopupMenuMode
)

var (
	procGetGUIThreadInfoSource = user32.NewProc("GetGUIThreadInfo")
	procGetAsyncKeyStateSource = user32.NewProc("GetAsyncKeyState")
	procCancelMenuSource       = user32.NewProc("SendMessageTimeoutW")
)

var sourceCancelMenu = func(ctx context.Context, owner uintptr) bool {
	if owner == 0 || ctx.Err() != nil {
		return false
	}
	timeout := 150 * time.Millisecond
	if deadline, ok := ctx.Deadline(); ok && time.Until(deadline) < timeout {
		timeout = time.Until(deadline)
	}
	if timeout < time.Millisecond {
		return false
	}
	var result uintptr
	sent, _, _ := procCancelMenuSource.Call(owner, 0x001f, 0, 0, 0x23, uintptr(timeout/time.Millisecond), uintptr(unsafe.Pointer(&result)))
	return sent != 0
}

// Menu dismissal is bounded and best effort. Never proceed solely because the
// message was delivered: the menu loop may exit after the window procedure.
func dismissBoundaryMenu(ctx context.Context, info sourceGUIThreadInfo) (sourceGUIThreadInfo, bool, error) {
	if info.MenuOwner == 0 || info.Flags&guiInMoveSize != 0 || info.MoveSize != 0 || sourceAnyInputPressed() {
		return info, false, nil
	}
	if !sourceCancelMenu(ctx, info.MenuOwner) {
		return info, false, nil
	}
	wait, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	for {
		next := sourceGUIThreadInfo{Size: uint32(unsafe.Sizeof(sourceGUIThreadInfo{}))}
		if err := sourceForegroundGUIThreadInfo(&next); err != nil {
			return info, false, err
		}
		if next.Active != info.Active {
			return next, false, nil
		}
		if next.Flags&guiMenuModeMask == 0 {
			return next, true, nil
		}
		if next.MenuOwner != info.MenuOwner {
			return next, false, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-wait.Done():
			timer.Stop()
			return next, false, nil
		case <-timer.C:
		}
	}
}

// Some custom context menus hold mouse capture without entering the native
// Win32 menu loop. Cancel only an idle capture, then prove that the same
// foreground window released it before allowing the boundary crossing.
func dismissBoundaryCapture(ctx context.Context, info sourceGUIThreadInfo) (sourceGUIThreadInfo, bool, error) {
	if info.Capture == 0 || info.Flags&(guiInMoveSize|guiMenuModeMask) != 0 || info.MoveSize != 0 || sourceAnyInputPressed() {
		return info, false, nil
	}
	if !sourceCancelMenu(ctx, info.Capture) {
		return info, false, nil
	}
	wait, cancel := context.WithTimeout(ctx, 150*time.Millisecond)
	defer cancel()
	for {
		next := sourceGUIThreadInfo{Size: uint32(unsafe.Sizeof(sourceGUIThreadInfo{}))}
		if err := sourceForegroundGUIThreadInfo(&next); err != nil {
			return info, false, err
		}
		if next.Active != info.Active || next.Flags&(guiInMoveSize|guiMenuModeMask) != 0 || next.MoveSize != 0 {
			return next, false, nil
		}
		if next.Capture == 0 {
			return next, true, nil
		}
		if next.Capture != info.Capture {
			return next, false, nil
		}
		timer := time.NewTimer(10 * time.Millisecond)
		select {
		case <-wait.Done():
			timer.Stop()
			return next, false, nil
		case <-timer.C:
		}
	}
}

type sourceGUIThreadInfo struct {
	Size      uint32
	Flags     uint32
	Active    uintptr
	Focus     uintptr
	Capture   uintptr
	MenuOwner uintptr
	MoveSize  uintptr
	Caret     uintptr
	CaretRect sourceGUIThreadRect
}

type sourceGUIThreadRect struct {
	Left   int32
	Top    int32
	Right  int32
	Bottom int32
}

var sourceForegroundGUIThreadInfo = func(info *sourceGUIThreadInfo) error {
	result, _, callErr := procGetGUIThreadInfoSource.Call(0, uintptr(unsafe.Pointer(info)))
	if result != 0 {
		return nil
	}
	if callErr == syscall.Errno(0) {
		return errors.New("GetGUIThreadInfo returned no extended error")
	}
	return callErr
}

var sourceAnyInputPressed = func() bool {
	for virtualKey := uintptr(1); virtualKey <= 0xff; virtualKey++ {
		state, _, _ := procGetAsyncKeyStateSource.Call(virtualKey)
		if uint16(state)&0x8000 != 0 {
			return true
		}
	}
	return false
}

func (hook *SourceHook) AdmitBoundary(ctx context.Context, _ control.BoundaryIntent) (peer.BoundaryAdmissionDecision, error) {
	if hook == nil {
		return 0, errors.New("source hook is required")
	}
	if ctx == nil {
		return 0, errors.New("boundary admission context is required")
	}
	if err := ctx.Err(); err != nil {
		return 0, err
	}
	if hook.boundaryGuard != nil {
		if err := hook.boundaryGuard(); err != nil {
			return 0, err
		}
	}
	info := sourceGUIThreadInfo{Size: uint32(unsafe.Sizeof(sourceGUIThreadInfo{}))}
	if err := sourceForegroundGUIThreadInfo(&info); err != nil {
		return 0, boundaryInspectionUnavailable(ctx, "inspect foreground GUI thread", err)
	}
	if info.Flags&guiMenuModeMask != 0 {
		updated, closed, err := dismissBoundaryMenu(ctx, info)
		if err != nil {
			return 0, boundaryInspectionUnavailable(ctx, "verify menu dismissal", err)
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if !closed {
			return peer.BoundaryAdmissionBlockedLocalMenu, nil
		}
		info = updated
	}
	if info.Capture != 0 {
		updated, released, err := dismissBoundaryCapture(ctx, info)
		if err != nil {
			return 0, boundaryInspectionUnavailable(ctx, "verify mouse capture dismissal", err)
		}
		if err := ctx.Err(); err != nil {
			return 0, err
		}
		if !released {
			return peer.BoundaryAdmissionBlockedMouseCapture, nil
		}
		info = updated
	}
	if info.Flags&guiInMoveSize != 0 || info.MoveSize != 0 {
		return peer.BoundaryAdmissionBlockedMoveSize, nil
	}
	if sourceAnyInputPressed() {
		return peer.BoundaryAdmissionBlockedPressedInput, nil
	}
	if hook.boundaryGuard != nil {
		if err := hook.boundaryGuard(); err != nil {
			return 0, err
		}
	}
	return peer.BoundaryAdmissionAllowed, nil
}

// This marker applies only to failed GUI reads, never boundaryGuard or a
// cancelled session. Failed inspection cannot authorize a crossing.
func boundaryInspectionUnavailable(ctx context.Context, operation string, cause error) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return fmt.Errorf("%s: %w", operation, errors.Join(peer.ErrBoundaryInspectionUnavailable, cause))
}
