//go:build windows

package appinstall

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
	"golang.org/x/sys/windows/registry"
	"io"
	"os"
	"path/filepath"
	"slices"
)

func directory(base string) (*securefs.Directory, string, error) {
	launch, err := productidentity.PrepareLaunch(base)
	if err != nil {
		return nil, "", err
	}
	launch.Close()
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return nil, "", err
	}
	parent, err := securefs.OpenDirectory(paths.Root)
	if err != nil {
		return nil, "", err
	}
	defer parent.Close()
	path := filepath.Join(paths.Root, "app")
	dir, err := parent.CreateDirectory("app", 0700)
	if errors.Is(err, os.ErrExist) {
		if _, err = peercredentials.ValidateSecurityDirectory(path); err != nil {
			return nil, "", err
		}
		dir, err = securefs.OpenReadDirectory(path)
	} else if err == nil {
		err = peercredentials.ProtectSecurityDirectoryHandle(dir.Handle())
		// os.OpenRoot does not share DELETE on Windows. Keep a verified
		// read handle after setting the ACL so staging does not conflict with
		// our own DELETE-capable creation handle. Child writes/removals still
		// use securefs handles relative to this directory.
		if err == nil {
			err = dir.Close()
			if err == nil {
				dir, err = securefs.OpenReadDirectory(path)
			}
		}
	}
	if err != nil {
		if dir != nil {
			dir.Close()
		}
		return nil, "", err
	}
	return dir, path, nil
}

func replace(dir *securefs.Directory, name string, data []byte) error {
	f, temp, err := dir.CreateTemporary(".setup-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(temp)
	if err = peercredentials.ProtectSecurityFileHandle(f); err == nil {
		_, err = f.Write(data)
	}
	if err == nil {
		err = f.Sync()
	}
	err = errors.Join(err, f.Close())
	if err != nil {
		return err
	}
	return dir.Replace(temp, name)
}
func read(dir *securefs.Directory, name string, limit int64) ([]byte, error) {
	f, err := dir.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, limit+1))
	if err != nil || int64(len(data)) > limit {
		return nil, ErrInvalid
	}
	return data, nil
}

// Install stages immutable, verified bytes before changing any launch pointer.
// Existing binaries and user identity/settings are never overwritten or deleted.
func Install(base, self string, p Package) (string, error) {
	if p.Bundle == nil {
		return "", ErrInvalid
	}
	settings, err := appsettings.Load(base)
	if err != nil {
		return "", err
	}
	lease, err := agentinstance.AcquireControlStartup()
	if err != nil {
		return "", ErrBusy
	}
	defer lease.Close()
	meta := p.Bundle.Metadata()
	if err := validateInstallHistory(base, meta.Generation); err != nil {
		return "", err
	}
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return "", err
	}
	if err := validateInstallRegistrationAt(registry.CURRENT_USER, uninstallKey, filepath.Join(paths.Root, "app")); err != nil {
		return "", err
	}
	if err := appsettings.ValidateStartupRegistration(base); err != nil {
		return "", err
	}
	dir, path, err := directory(base)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	r := Receipt{Schema: Schema, Generation: meta.Generation, Revision: meta.Revision, Folders: []string{p.ID}}
	if data, e := read(dir, "installation.json", 8192); e == nil {
		old, e := DecodeReceipt(data)
		if e != nil {
			return "", e
		}
		if old.Generation > meta.Generation {
			return "", ErrOlder
		}
		for _, name := range old.Folders {
			if !slices.Contains(r.Folders, name) {
				r.Folders = append(r.Folders, name)
			}
		}
	} else if !errors.Is(e, os.ErrNotExist) {
		return "", e
	}
	if len(r.Folders) > 64 {
		return "", ErrInvalid
	}
	root, err := os.OpenRoot(path)
	if err != nil {
		return "", err
	}
	err = p.Bundle.Stage(root, p.ID)
	root.Close()
	if err != nil && !errors.Is(err, os.ErrExist) {
		return "", err
	}
	installed := filepath.Join(path, p.ID)
	guard, err := securefs.OpenReadDirectory(installed)
	if err != nil {
		return "", err
	}
	err = p.Bundle.MatchDirectory(guard)
	guard.Close()
	if err != nil {
		// Repair into a fresh folder. Never overwrite corrupt or running images.
		var id [16]byte
		if _, err = rand.Read(id[:]); err != nil {
			return "", err
		}
		p.ID = hex.EncodeToString(id[:])
		root, err = os.OpenRoot(path)
		if err != nil {
			return "", err
		}
		err = p.Bundle.Stage(root, p.ID)
		root.Close()
		if err != nil {
			return "", err
		}
		r.Folders = append([]string{p.ID}, r.Folders...)
		if len(r.Folders) > 64 {
			return "", ErrInvalid
		}
		installed = filepath.Join(path, p.ID)
		guard, err = securefs.OpenReadDirectory(installed)
		if err != nil {
			return "", err
		}
		err = p.Bundle.MatchDirectory(guard)
		guard.Close()
		if err != nil {
			return "", err
		}
	}
	// Store the signed/unsigned self-contained setup, not a shell or remote path.
	source, err := securefs.OpenReadDirectory(filepath.Dir(self))
	if err != nil {
		return "", err
	}
	data, err := read(source, filepath.Base(self), 64<<20)
	source.Close()
	if err != nil {
		return "", err
	}
	if old, e := read(dir, "krasis-setup.exe", 64<<20); e != nil || sha256.Sum256(old) != sha256.Sum256(data) {
		if err = replace(dir, "krasis-setup.exe", data); err != nil {
			return "", err
		}
	}
	body, _ := json.Marshal(r)
	// Receipt precedes registrations, so interrupted setup remains repairable.
	if err = replace(dir, "installation.json", body); err != nil {
		return "", err
	}
	ui := filepath.Join(installed, "krasis.exe")
	if err = registerInstalledLaunch(base, ui, meta.Generation, meta.Revision, settings.StartAtLogin, appsettings.SetRecoveryTask, appsettings.SetStartup); err != nil {
		return "", err
	}
	if err = launchrecord.RegisterStartMenu(base); err != nil {
		return "", err
	}
	if err = registerUninstall(path, meta.Generation); err != nil {
		return "", err
	}
	return ui, nil
}

func validateInstallHistory(base string, generation int) error {
	current, err := launchrecord.Load(base)
	if err != nil && !errors.Is(err, launchrecord.ErrMissing) {
		return err
	}
	if err == nil && current.Generation > generation {
		return ErrOlder
	}
	// A bad previous record also prevents recovery-task migration. Check it
	// before touching application images, while permitting image-only repair.
	_, err = launchrecord.RecoveryDirectories(base)
	return err
}
