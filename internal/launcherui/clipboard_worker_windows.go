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
func RunClipboardWorker(ctx context.Context, control string, owner *atomic.Uintptr, gate *sync.Mutex, revision *atomic.Uint64) {
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
	var lastOwner uintptr
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
			hwnd := clipboardOwnerWindow(owner)
			if hwnd == 0 {
				pump = clipboardPump{}
				lastOwner = 0
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
			if hwnd != lastOwner {
				pump = clipboardPump{}
				lastOwner = hwnd
			}
			if currentRevision := revision.Load(); currentRevision != lastRevision {
				pump = clipboardPump{}
				lastRevision = currentRevision
			}
			tick, cancel := context.WithTimeout(ctx, 60*time.Second)
			clipboardWorkerTick(tick, control, hwnd, &pump)
			cancel()
			gate.Unlock()
		}
	}
}

// The host publishes its real UI handle once ready. Titles and visibility are
// presentation state; neither may select a clipboard owner or reset its baseline.
func clipboardOwnerWindow(owner *atomic.Uintptr) uintptr {
	if owner == nil {
		return 0
	}
	hwnd := owner.Load()
	if hwnd == 0 {
		return 0
	}
	if live, _, _ := clipUser.NewProc("IsWindow").Call(hwnd); live == 0 {
		return 0
	}
	var pid uint32
	clipUser.NewProc("GetWindowThreadProcessId").Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid != uint32(os.Getpid()) {
		return 0
	}
	return hwnd
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
