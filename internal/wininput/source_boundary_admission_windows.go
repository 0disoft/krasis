//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"syscall"
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
)

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
	info := sourceGUIThreadInfo{Size: uint32(unsafe.Sizeof(sourceGUIThreadInfo{}))}
	if err := sourceForegroundGUIThreadInfo(&info); err != nil {
		return 0, fmt.Errorf("inspect foreground GUI thread: %w", err)
	}
	if info.Flags&guiMenuModeMask != 0 {
		return peer.BoundaryAdmissionBlockedLocalMenu, nil
	}
	if info.Capture != 0 {
		return peer.BoundaryAdmissionBlockedMouseCapture, nil
	}
	if info.Flags&guiInMoveSize != 0 || info.MoveSize != 0 {
		return peer.BoundaryAdmissionBlockedMoveSize, nil
	}
	if sourceAnyInputPressed() {
		return peer.BoundaryAdmissionBlockedPressedInput, nil
	}
	return peer.BoundaryAdmissionAllowed, nil
}
