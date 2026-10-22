//go:build windows

package appinstall

import (
	"context"
	"crypto/sha256"
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
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, ErrInvalid
	}
	return data, nil
}

// Install stages immutable, verified bytes before changing any launch pointer.
// Existing binaries and user identity/settings are never overwritten or deleted.
func Install(base, self string, p Package) (string, error) {
	return install(context.Background(), base, self, p, nil)
}

// InstallWithShutdown holds the startup fence from preflight through publication.
// Scheduled recovery and the old UI cannot start another agent during setup.
func InstallWithShutdown(ctx context.Context, base, self string, p Package) (string, error) {
	return install(ctx, base, self, p, func() (io.Closer, error) {
		return stopForSetup(ctx, base, p.Bundle.Metadata().Generation, p.Bundle.Metadata().Revision)
	})
}

func install(ctx context.Context, base, self string, p Package, before func() (io.Closer, error)) (ui string, err error) {
	stage := "preflight"
	defer func() { err = installationFailure(stage, err) }()
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if p.Bundle == nil {
		return "", ErrInvalid
	}
	stage = "settings"
	if _, err := appsettings.Load(base); err != nil {
		return "", err
	}
	stage = "startup_lock"
	lease, err := agentinstance.AcquireControlStartup()
	if before != nil && errors.Is(err, agentinstance.ErrAlreadyRunning) {
		lease, err = waitSetupStartup(ctx)
	}
	if err != nil {
		if before != nil {
			return "", &shutdownError{"startup_lock", err}
		}
		return "", ErrBusy
	}
	defer lease.Close()
	meta := p.Bundle.Metadata()
	stage = "launch_history"
	if err := validateInstallHistory(base, meta.Generation); err != nil {
		return "", err
	}
	stage = "product_paths"
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return "", err
	}
	stage = "app_registration"
	if err := validateInstallRegistrationAt(registry.CURRENT_USER, uninstallKey, filepath.Join(paths.Root, "app")); err != nil {
		return "", err
	}
	stage = "startup_registration"
	if err := appsettings.ValidateStartupRegistration(base); err != nil {
		return "", err
	}
	stage = "recovery_registration"
	if err := appsettings.ValidateRecoveryTaskRegistration(base); err != nil {
		return "", err
	}
	stage = "install_directory"
	dir, path, err := directory(base)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	stage = "install_plan"
	plan, err := prepareInstallPlan(ctx, dir, self, Receipt{Schema: Schema, Generation: meta.Generation, Revision: meta.Revision, Folders: []string{p.ID}},
		func(next Receipt) (Receipt, error) { return prepareInstallFolder(path, p, next) },
		func(next Receipt) error { return stageInstallImage(path, p, next.Folders[0]) }, before)
	if err != nil {
		return "", err
	}
	if plan.idle != nil {
		defer plan.idle.Close()
	}
	// The old GUI could change preferences during image preparation. It is now
	// stopped; pin fresh settings through OS registration and publication.
	stage = "settings_refresh"
	preferences, settings, err := lockInstalledPreferences(base)
	if err != nil {
		return "", err
	}
	defer preferences.Close()
	r := plan.receipt
	p.ID = r.Folders[0]
	installed := filepath.Join(path, p.ID)
	stage = "verify_image"
	if err = verifyInstallImage(installed, p); err != nil {
		// The repair location was fixed before shutdown. A later change is not
		// permission to create another unplanned directory or overwrite bytes.
		return "", err
	}
	// Self bytes were already read before stopping the running product.
	stage = "setup_image"
	data := plan.setup
	if old, e := read(dir, "krasis-setup.exe", 64<<20); e != nil || sha256.Sum256(old) != sha256.Sum256(data) {
		if err = replace(dir, "krasis-setup.exe", data); err != nil {
			return "", err
		}
	}
	body, _ := json.Marshal(r)
	// Receipt precedes registrations, so interrupted setup remains repairable.
	stage = "installation_receipt"
	if err = replace(dir, "installation.json", body); err != nil {
		return "", err
	}
	ui = filepath.Join(installed, "krasis.exe")
	stage = "launch_registration"
	if err = registerInstalledLaunch(base, ui, meta.Generation, meta.Revision, settings.StartAtLogin, appsettings.SetRecoveryTask, appsettings.SetStartup); err != nil {
		return "", err
	}
	stage = "start_menu"
	if err = launchrecord.RegisterStartMenu(base); err != nil {
		return "", err
	}
	stage = "uninstall_registration"
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
