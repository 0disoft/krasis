//go:build windows

package wininput

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"unsafe"

	"github.com/0disoft/krasis/internal/winmessage"
	"golang.org/x/sys/windows"
)

var procSourceMessageWait = user32.NewProc("MsgWaitForMultipleObjectsEx")

const (
	sourceMessageCancelFallbackMillis = winmessage.MaximumWaitMillis
	sourceMessageQueueAllInput        = 0x04ff
	sourceMessageInputAvailable       = 0x0004
	sourceMessageRemove               = 1
)

// A kernel event wakes cancellation independently of desktop message delivery.
// Only this owner closes it, after joining the signaler. Check cancellation
// between messages too: a busy queue must not starve shutdown behind WM_QUIT.
func sourceMessageLoop(ctx context.Context, visit func(message) error) (resultErr error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	wake, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return sourceStage("message-loop", err)
	}
	stop, stopped := make(chan struct{}), make(chan struct{})
	var signalErr error
	go func() {
		defer close(stopped)
		select {
		case <-ctx.Done():
			signalErr = windows.SetEvent(wake)
		case <-stop:
		}
	}()
	defer func() {
		close(stop)
		<-stopped
		resultErr = errors.Join(resultErr, sourceStage("message-loop", signalErr), sourceStage("message-loop", windows.CloseHandle(wake)))
	}()
	var msg message
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		// Normally the event wakes immediately. A finite fallback bounds even
		// SetEvent failure without mistaking a missing WM_QUIT for a live owner.
		state, _, waitErr := procSourceMessageWait.Call(1, uintptr(unsafe.Pointer(&wake)), sourceMessageCancelFallbackMillis, sourceMessageQueueAllInput, sourceMessageInputAvailable)
		runtime.KeepAlive(wake)
		switch uint32(state) {
		case windows.WAIT_OBJECT_0:
			return ctx.Err()
		case uint32(windows.WAIT_TIMEOUT):
			continue
		case windows.WAIT_OBJECT_0 + 1: // QS_ALLINPUT and MWMO_INPUTAVAILABLE.
			if err := ctx.Err(); err != nil {
				return err
			}
			available, _, _ := procPeekMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0, sourceMessageRemove)
			if available == 0 {
				continue // PeekMessage may have serviced a sent/system message.
			}
			if msg.Message == wmQuit {
				return nil
			}
			if visit != nil {
				if err := visit(msg); err != nil {
					return err
				}
			}
			procTranslateMessage.Call(uintptr(unsafe.Pointer(&msg)))
			procDispatchMessageW.Call(uintptr(unsafe.Pointer(&msg)))
		default:
			return sourceStage("message-loop", fmt.Errorf("message wait returned %d: %w", state, waitErr))
		}
	}
}
