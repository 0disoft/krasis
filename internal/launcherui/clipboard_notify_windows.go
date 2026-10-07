//go:build windows

package launcherui

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"time"
)

const ClipboardUpdateMessage = 0x031d

// Called only by a real owner-window notification. It never reads contents,
// performs I/O or waits inside the GUI's native window procedure.
func WakeClipboard(wake chan<- struct{}) {
	if wake == nil {
		return
	}
	select {
	case wake <- struct{}{}:
	default:
	}
}
func ListenClipboardChanges(window uintptr, enable bool) bool {
	if window == 0 {
		return false
	}
	name := "AddClipboardFormatListener"
	if !enable {
		name = "RemoveClipboardFormatListener"
	}
	ok, _, _ := clipUser.NewProc(name).Call(window)
	return ok != 0
}

// Agent long-poll wakes the owner immediately when a remote payload is ready.
// Counters only; data stays in the existing authenticated clipboard API.
func watchAgentClipboard(ctx context.Context, control string, wake chan<- struct{}) {
	var revision uint64
	for ctx.Err() == nil {
		client, err := localapi.OpenClient(control)
		if err == nil {
			attempt, cancel := context.WithTimeout(ctx, 3*time.Second)
			value, waitErr := client.WaitClipboard(attempt, revision)
			cancel()
			if waitErr == nil && value.Code == "ok" {
				if value.Revision != revision {
					revision = value.Revision
					WakeClipboard(wake)
				}
				continue
			}
		}
		// Missing/legacy agents keep the existing periodic fallback. Never retry
		// unavailable APIs in a tight loop or start/recover the agent here.
		timer := time.NewTimer(750 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
