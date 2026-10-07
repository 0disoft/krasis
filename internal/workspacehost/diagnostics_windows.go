//go:build windows && desktop

package workspacehost

import (
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/windows"
)

func (s *WorkspaceService) DiagnosticsStatus() productpanel.DiagnosticsSnapshot {
	return s.diagnostics.Status()
}
func (s *WorkspaceService) DiagnosticsExport(peer string) productpanel.DiagnosticsSnapshot {
	return s.diagnostics.Export(peer)
}
func (s *WorkspaceService) DiagnosticsCollect() productpanel.DiagnosticsSnapshot {
	return s.diagnostics.BeginCollect()
}
func (s *WorkspaceService) DiagnosticsOpen() productpanel.DiagnosticsSnapshot {
	return s.diagnostics.OpenFolder()
}

func diagnosticFiles(base string) (bool, bool, error) {
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return false, false, err
	}
	dir, err := securefs.OpenDirectory(paths.Diagnostics)
	if errors.Is(err, os.ErrNotExist) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	defer dir.Close()
	// Prefer this collection's native-derived files, not an older export.
	if receipt, err := productlog.LoadCollectionReceipt(base); err != nil {
		return false, false, err
	} else if receipt != nil {
		return receipt.LocalSaved, receipt.PeerSaved, nil
	}
	exists := func(name string) bool {
		f, err := dir.Open(name)
		if err != nil {
			return false
		}
		defer f.Close()
		info, err := f.Stat()
		return err == nil && info.Mode().IsRegular()
	}
	saved := exists(productlog.ExportName)
	// Reuse the validated directory handle. Opening the path again through
	// os.Open conflicts with a live diagnostic writer's Windows sharing mode.
	var handle windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), windows.Handle(dir.Handle()), windows.CurrentProcess(), &handle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return saved, false, err
	}
	listing := os.NewFile(uintptr(handle), paths.Diagnostics)
	defer listing.Close()
	// Bound directory enumeration; open candidate files through securefs only.
	for batch := 0; batch < 16; batch++ {
		names, err := listing.Readdirnames(256)
		for _, name := range names {
			if !strings.HasPrefix(name, "support-peer-") || !strings.HasSuffix(name, ".json") {
				continue
			}
			id := strings.TrimSuffix(strings.TrimPrefix(name, "support-peer-"), ".json")
			decoded, decodeErr := hex.DecodeString(id)
			if decodeErr == nil && len(decoded) == 32 && exists(name) {
				return saved, true, nil
			}
		}
		if errors.Is(err, io.EOF) {
			return saved, false, nil
		}
		if err != nil {
			return saved, false, err
		}
	}
	return saved, false, errors.New("diagnostic directory limit")
}

func openDiagnosticsFolder(base string) error {
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return err
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	// Even a corrupt collection pointer must leave the folder accessible for
	// diagnosis. Selection is optional; only a validated native file is used.
	if selected, err := productlog.CollectionLocalPath(base); err == nil && selected != "" {
		verb, _ := windows.UTF16PtrFromString("open")
		explorer, err := windows.UTF16PtrFromString(filepath.Join(os.Getenv("WINDIR"), "explorer.exe"))
		if err != nil {
			return err
		}
		args, err := windows.UTF16PtrFromString(`/select,"` + selected + `"`)
		if err != nil {
			return err
		}
		return windows.ShellExecute(0, verb, explorer, args, nil, windows.SW_SHOWNORMAL)
	}
	verb, _ := windows.UTF16PtrFromString("open")
	path, err := windows.UTF16PtrFromString(paths.Diagnostics)
	if err != nil {
		return err
	}
	// Only this native-derived directory is opened, never a renderer path/URL.
	return windows.ShellExecute(0, verb, path, nil, nil, windows.SW_SHOWNORMAL)
}
