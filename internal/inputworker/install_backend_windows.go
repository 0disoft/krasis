//go:build windows

package inputworker

import (
	"context"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// Only the attended embedded-payload installer connects this private backend.
// Receiving an update bundle cannot elevate an ordinary agent into its service.
type windowsInstallBackend struct {
	files *installFiles
	scm   *installSCM
}

var _ installBackend = (*windowsInstallBackend)(nil)

func newWindowsInstallBackend(payload *installPayload) (*windowsInstallBackend, error) {
	files, err := newInstallFiles(payload)
	if err != nil {
		return nil, err
	}
	manager, err := windows.OpenSCManager(nil, nil, windows.SC_MANAGER_CONNECT|windows.SC_MANAGER_CREATE_SERVICE)
	if err != nil {
		_ = files.close()
		return nil, ErrAuthority
	}
	return &windowsInstallBackend{files: files, scm: &installSCM{manager: manager, root: files.root}}, nil
}

func (backend *windowsInstallBackend) Close() error {
	if backend == nil {
		return nil
	}
	var result error
	if backend.scm != nil && backend.scm.manager != 0 {
		if windows.CloseServiceHandle(backend.scm.manager) != nil {
			result = ErrAuthority
		}
		backend.scm.manager = 0
	}
	if backend.files != nil && backend.files.close() != nil {
		result = ErrAuthority
	}
	return result
}

func (backend *windowsInstallBackend) check(ctx context.Context) error {
	if backend == nil || backend.files == nil || backend.scm == nil || backend.scm.manager == 0 {
		return ErrAuthority
	}
	return backend.files.check(ctx)
}

func (backend *windowsInstallBackend) Lock(ctx context.Context) (func(), error) {
	if backend == nil || backend.files == nil || backend.scm == nil || backend.scm.manager == 0 {
		return nil, ErrAuthority
	}
	return backend.files.acquire(ctx)
}

func (backend *windowsInstallBackend) Load(ctx context.Context) (*installJournal, error) {
	if err := backend.check(ctx); err != nil {
		return nil, err
	}
	return backend.files.load(ctx)
}

func (backend *windowsInstallBackend) Snapshot(ctx context.Context) (installSnapshot, error) {
	if err := backend.check(ctx); err != nil {
		return installSnapshot{}, err
	}
	status, present, err := backend.scm.query(ctx)
	if err != nil {
		return installSnapshot{}, err
	}
	if missingInstallPath(backend.files.directory(installActive)) {
		if present {
			return installSnapshot{}, ErrAuthority
		}
		return installSnapshot{}, nil
	}
	if !present {
		return installSnapshot{}, ErrAuthority
	}
	images, err := backend.images(ctx, installActive)
	if err != nil {
		return installSnapshot{}, err
	}
	state := installSnapshot{Present: true, Images: images}
	switch status.CurrentState {
	case windows.SERVICE_RUNNING:
		if backend.scm.ready(status, images.Service) != nil {
			return installSnapshot{}, ErrAuthority
		}
		state.Running = true
	case windows.SERVICE_STOPPED:
		if backend.scm.cleanStopped(status, images.Service, 0) != nil {
			return installSnapshot{}, ErrRecovery
		}
	default:
		return installSnapshot{}, ErrAuthority
	}
	return state, nil
}

func (backend *windowsInstallBackend) images(ctx context.Context, kind installDirectory) (installImages, error) {
	installation, err := backend.files.readImages(ctx, kind, nil)
	if err != nil {
		return installImages{}, err
	}
	images := installImages{Agent: installation.agentDigest, Service: installation.serviceDigest}
	if installation.Close() != nil {
		return installImages{}, ErrAuthority
	}
	return images, nil
}

func (backend *windowsInstallBackend) Save(ctx context.Context, journal installJournal) error {
	if err := backend.check(ctx); err != nil {
		return err
	}
	return backend.files.save(ctx, journal)
}

func (backend *windowsInstallBackend) Stage(ctx context.Context, journal installJournal) error {
	if err := backend.check(ctx); err != nil {
		return err
	}
	return backend.files.stage(ctx, journal)
}

func (backend *windowsInstallBackend) Stop(ctx context.Context) error {
	if err := backend.check(ctx); err != nil {
		return err
	}
	_, present, err := backend.scm.query(ctx)
	if err != nil || !present {
		return err
	}
	// Replacement can be interrupted between the two directory renames. The
	// stopped service still belongs to the pinned retired/rollback image then.
	for _, kind := range []installDirectory{installActive, installRetired, installBackup} {
		if missingInstallPath(backend.files.directory(kind)) {
			continue
		}
		images, err := backend.images(ctx, kind)
		if err != nil {
			return err
		}
		return backend.scm.stop(ctx, images.Service)
	}
	return ErrAuthority
}

func (backend *windowsInstallBackend) Replace(ctx context.Context, target installImages) error {
	if err := backend.check(ctx); err != nil {
		return err
	}
	if err := backend.files.replace(ctx, target); err != nil {
		return err
	}
	return backend.scm.configure(ctx, target.valid())
}

func (backend *windowsInstallBackend) Restore(ctx context.Context, previous installSnapshot) error {
	if err := backend.check(ctx); err != nil {
		return err
	}
	if err := backend.files.restore(ctx, previous); err != nil {
		return err
	}
	return backend.scm.configure(ctx, previous.Present)
}

func (backend *windowsInstallBackend) Start(ctx context.Context) error {
	if err := backend.check(ctx); err != nil {
		return err
	}
	images, err := backend.images(ctx, installActive)
	if err != nil {
		return err
	}
	return backend.scm.start(ctx, images.Service)
}

func (backend *windowsInstallBackend) Verify(ctx context.Context, expected installSnapshot) error {
	if err := backend.check(ctx); err != nil || !expected.valid() {
		return ErrAuthority
	}
	actual, err := backend.Snapshot(ctx)
	if err != nil || actual != expected {
		return ErrAuthority
	}
	return nil
}

func (backend *windowsInstallBackend) Finish(ctx context.Context) error {
	if err := backend.check(ctx); err != nil {
		return err
	}
	if missingInstallPath(backend.files.directory(installActive)) {
		_, present, err := backend.scm.query(ctx)
		if err != nil || present {
			return ErrAuthority
		}
		// Delete lifecycle evidence before the transaction journal, so an
		// interrupted removal remains recoverable.
		for _, name := range []string{"INPUT-SERVICE.pending", "INPUT-SERVICE.record"} {
			if err := removeInstallFile(ctx, filepath.Join(backend.files.root, name)); err != nil {
				return err
			}
		}
	}
	if err := backend.files.finish(ctx); err != nil {
		return err
	}
	return nil
}
