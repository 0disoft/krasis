//go:build windows

package appinstall

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// Remove preserves pairing, credentials, preferences and diagnostics. Unknown
// files, processes or ownership prevent removal; no process is force-killed.
func Remove(ctx context.Context, base string) error {
	lease, err := agentinstance.AcquireControlStartup()
	if err != nil {
		return ErrBusy
	}
	defer lease.Close()
	live, err := launchrecord.LiveDirectories()
	if err != nil || len(live) != 0 {
		return ErrBusy
	}
	dir, path, err := directory(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	data, err := read(dir, "installation.json", 8192)
	if err != nil {
		return err
	}
	r, err := DecodeReceipt(data)
	if err != nil {
		return err
	}
	if err = validateUninstall(path); err != nil {
		return err
	}
	// Preflight the entire bounded deletion set before changing registrations.
	for _, name := range r.Folders {
		guard, e := securefs.OpenDirectory(filepath.Join(path, name))
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		names, e := guard.ReadNames()
		if e == nil {
			for _, leaf := range names {
				if !slices.Contains(files, leaf) {
					e = ErrInvalid
					break
				}
				f, openErr := guard.Open(leaf)
				if openErr != nil {
					e = openErr
					break
				}
				f.Close()
			}
		}
		guard.Close()
		if e != nil {
			return e
		}
	}
	programFiles, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return err
	}
	if _, e := os.Lstat(filepath.Join(programFiles, "Krasis", "Input")); e == nil {
		current, e := launchrecord.Inspect(base)
		if e != nil {
			return e
		}
		if e = productpanel.RemoveProtectedInput(ctx, filepath.Dir(current.Executable), current.Revision); e != nil {
			return e
		}
		if _, e = os.Lstat(filepath.Join(programFiles, "Krasis", "Input")); !errors.Is(e, os.ErrNotExist) {
			return ErrInvalid
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return e
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	if err = appsettings.SetRecoveryTask(base, false); err != nil {
		return err
	}
	if err = appsettings.SetStartup(base, false); err != nil {
		return err
	}
	v, err := appsettings.Load(base)
	if err != nil {
		return err
	}
	v.StartAtLogin = false
	if err = appsettings.Save(base, v); err != nil {
		return err
	}
	if err = launchrecord.UnregisterStartMenu(base); err != nil {
		return err
	}
	for _, name := range r.Folders {
		guard, e := securefs.OpenDirectory(filepath.Join(path, name))
		if errors.Is(e, os.ErrNotExist) {
			continue
		}
		if e != nil {
			return e
		}
		for _, leaf := range files {
			if e = guard.Remove(leaf); e != nil && !errors.Is(e, os.ErrNotExist) {
				guard.Close()
				return e
			}
		}
		e = guard.RemoveSelf()
		guard.Close()
		if e != nil {
			return e
		}
	}
	launch, err := launchrecord.Directory(base)
	if err != nil {
		return err
	}
	guard, err := securefs.OpenDirectory(launch)
	if err != nil {
		return err
	}
	// Keep launch receipts for external portable versions rather than deleting
	// arbitrary generation folders mentioned by them.
	for _, leaf := range []string{"current.json", "previous.json"} {
		if leaf == "current.json" {
			current, e := launchrecord.Load(base)
			if e == nil && !strings.HasPrefix(current.Executable, path+string(filepath.Separator)) {
				continue
			}
		}
		if e := guard.Remove(leaf); e != nil && !errors.Is(e, os.ErrNotExist) {
			guard.Close()
			return e
		}
	}
	guard.Close()
	if err = dir.Remove("krasis-setup.exe"); err != nil {
		return err
	}
	if err = removeUninstall(path); err != nil {
		return err
	}
	return dir.Remove("installation.json")
}
