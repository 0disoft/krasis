//go:build windows

package launcherui

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"time"
)

const maxClipboardCleanupBatch = 4
const clipboardCleanupLockBudget = 20 * time.Millisecond

func cleanupClipboardFiles(base string) {
	// Directory enumeration and recursive removal must not hold OpenClipboard.
	candidates := clipboardCleanupCandidates(base)
	if len(candidates) == 0 {
		return
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	if !clipboardDesktopAvailable() {
		return
	}
	if ok, _, _ := clipOpen.Call(0); ok == 0 {
		return
	}
	opened := true
	defer func() {
		if opened {
			clipClose.Call()
		}
	}()
	var referenced []string
	if clipboardHasFormat(cfHDROP) {
		var err error
		referenced, err = clipboardDropPaths()
		if err != nil {
			return
		}
	}
	trash := quarantineClipboardCandidates(base, referenced, candidates)
	clipClose.Call()
	opened = false
	removeClipboardTrash(trash)
}

func clipboardCleanupCandidates(base string) []string {
	if !filepath.IsAbs(base) {
		return nil
	}
	f, err := os.Open(base)
	if err != nil {
		return nil
	}
	defer f.Close()
	var candidates []string
	// Bound enumeration as well as deletion. Later sweeps revisit remaining work.
	for scanned := 0; scanned < 512; scanned += 64 {
		items, err := f.ReadDir(64)
		for _, item := range items {
			if !item.IsDir() {
				continue
			}
			name := item.Name()
			if strings.HasPrefix(name, "delete-") {
				candidates = append(candidates, name)
			} else if strings.HasPrefix(name, "copy-") {
				info, statErr := item.Info()
				if statErr == nil && time.Since(info.ModTime()) > 24*time.Hour {
					candidates = append(candidates, name)
				}
			}
		}
		if err != nil {
			break
		}
	}
	return candidates
}

// While holding the clipboard, rename only unreferenced stale stages. Failed
// deletions remain under delete-* for the next sweep; no file content is logged.
func quarantineClipboardCandidates(base string, referenced, candidates []string) []string {
	if !filepath.IsAbs(base) {
		return nil
	}
	for _, path := range referenced {
		if !filepath.IsAbs(path) {
			return nil
		}
	}
	deadline := time.Now().Add(clipboardCleanupLockBudget)
	var trash []string
	for _, name := range candidates {
		if len(trash) >= maxClipboardCleanupBatch || time.Now().After(deadline) {
			break
		}
		if filepath.Base(name) != name || (!strings.HasPrefix(name, "copy-") && !strings.HasPrefix(name, "delete-")) {
			continue
		}
		stage := filepath.Join(base, name)
		inUse := false
		for _, path := range referenced {
			clean := filepath.Clean(path)
			if strings.EqualFold(clean, stage) || strings.HasPrefix(strings.ToLower(clean), strings.ToLower(stage+string(os.PathSeparator))) {
				inUse = true
				break
			}
		}
		if inUse {
			continue
		}
		if !safeClipboardStage(stage) {
			continue
		}
		if strings.HasPrefix(name, "copy-") {
			info, err := os.Stat(stage)
			if err != nil || time.Since(info.ModTime()) <= 24*time.Hour {
				continue
			}
			destination := filepath.Join(base, "delete-"+strings.TrimPrefix(name, "copy-"))
			if _, err = os.Lstat(destination); !os.IsNotExist(err) {
				continue
			}
			if err = os.Rename(stage, destination); err != nil {
				continue
			}
			stage = destination
		}
		trash = append(trash, stage)
	}
	return trash
}

func safeClipboardStage(path string) bool {
	wide, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return false
	}
	attrs, err := syscall.GetFileAttributes(wide)
	return err == nil && attrs&syscall.FILE_ATTRIBUTE_DIRECTORY != 0 && attrs&syscall.FILE_ATTRIBUTE_REPARSE_POINT == 0
}

func removeClipboardTrash(trash []string) {
	for _, path := range trash {
		if safeClipboardStage(path) {
			_ = os.RemoveAll(path)
		}
	}
}

func cleanupClipboardFilesWithReferences(base string, referenced []string) {
	removeClipboardTrash(quarantineClipboardCandidates(base, referenced, clipboardCleanupCandidates(base)))
}
