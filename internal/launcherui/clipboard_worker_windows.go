//go:build windows

package launcherui

import (
	"context"
	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/localapi"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

// RunClipboardWorker uses this process's UI window as clipboard owner. No clipboard
// text enters the renderer; this reuses the legacy sequence/marker protections.
// The caller cancels ctx and joins the worker before exiting.
func RunClipboardWorker(ctx context.Context, control, title string, gate *sync.Mutex, revision *atomic.Uint64) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	timer := time.NewTicker(750 * time.Millisecond)
	defer timer.Stop()
	cleanup := time.NewTicker(time.Hour)
	defer cleanup.Stop()
	if base, err := clipboardFilesRoot(); err == nil {
		cleanupClipboardFiles(base)
	}
	var pump clipboardPump
	var lastRevision uint64
	for {
		select {
		case <-ctx.Done():
			return
		case <-cleanup.C:
			if base, err := clipboardFilesRoot(); err == nil {
				cleanupClipboardFiles(base)
			}
		case <-timer.C:
			// The UI thread pumps owner-window notifications; a window on this
			// network worker thread would stall other applications' clipboard writes.
			name := utf16(title)
			hwnd, _, _ := clipUser.NewProc("FindWindowW").Call(0, uintptr(unsafe.Pointer(name)))
			runtime.KeepAlive(name)
			var pid uint32
			if hwnd != 0 {
				clipUser.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
			}
			if hwnd == 0 || pid != uint32(os.Getpid()) {
				pump = clipboardPump{}
				continue
			}
			// The legacy controller already owns a pump. On handover reset the baseline
			// so pre-existing clipboard contents are not published by the new owner.
			running, err := agentinstance.ControlRunning()
			if err != nil || running {
				pump = clipboardPump{}
				continue
			}
			gate.Lock()
			if currentRevision := revision.Load(); currentRevision != lastRevision {
				pump = clipboardPump{}
				lastRevision = currentRevision
			}
			tick, cancel := context.WithTimeout(ctx, 12*time.Second)
			clipboardWorkerTick(tick, control, hwnd, &pump)
			cancel()
			gate.Unlock()
		}
	}
}

func clipboardWorkerTick(ctx context.Context, control string, hwnd uintptr, pump *clipboardPump) {
	client, err := localapi.OpenClient(control)
	if err != nil {
		return
	}
	status, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
	if err != nil {
		return
	}
	peer := status.Status.ClipboardPeer
	if peer == "" {
		*pump = clipboardPump{}
		return
	}
	trust, err := client.Trust(ctx)
	if err != nil {
		*pump = clipboardPump{}
		return
	}
	fingerprints := map[string]string{}
	for _, p := range trust.Peers {
		if !p.Revoked {
			fingerprints[p.Fingerprint] = p.Fingerprint
		}
	}
	pump.sync(ctx, hwnd, control, peer, fingerprints)
}
