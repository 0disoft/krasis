//go:build windows

package inputworker

import (
	"context"
	"errors"
	"path/filepath"

	"golang.org/x/sys/windows"
)

// The installer lock is held. No journal or failed receipt is removed or
// rewritten here: only the verified service can publish a new clean receipt.
func (backend *windowsInstallBackend) repairFailedService(ctx context.Context, images installImages) (result error) {
	journal, err := backend.files.load(ctx)
	if err != nil {
		return err
	}
	if journal != nil {
		return ErrRecovery
	}
	installation, err := loadInstallationDirectory(backend.files.root, filepath.Join(backend.files.root, "Input"))
	if err != nil {
		return err
	}
	defer func() {
		if err := installation.Close(); err != nil {
			result = errors.Join(result, installNativeError("file_close", ErrRecovery, err))
		}
	}()
	if installation.agentDigest != images.Agent || installation.serviceDigest != images.Service {
		return ErrAuthority
	}
	record, err := readServiceRecord(backend.files.root)
	if err != nil {
		return err
	}
	return repairStoppedServiceTransaction(ctx, record, images, backend, retiredServiceProcess,
		func(ctx context.Context) error { return backend.scm.start(ctx, images.Service, false) },
		func(ctx context.Context) error { return backend.scm.stop(ctx, images.Service) })
}

// The installer holds its exclusive lock throughout repair. An older service
// refuses a normal start under that lock without a durable journal. Use its
// existing restored-image phase, bound to the independently pinned old images;
// do not change the old service's startup policy or release the install lock.
func repairStoppedServiceTransaction(ctx context.Context, record serviceRecord, images installImages, backend installBackend,
	retired func(uint32) (bool, error), start, stop func(context.Context) error) error {
	previous := installSnapshot{Present: true, Images: images}
	if !previous.valid() || backend == nil {
		return ErrRecovery
	}
	return repairStoppedService(ctx, record, images.Service, retired, func(ctx context.Context) error {
		journal := installJournal{Schema: "krasis.input.transaction/v1", Phase: installRollbackStart, Previous: previous, Target: images}
		if err := backend.Save(ctx, journal); err != nil {
			return errors.Join(errInstallJournal, err)
		}
		return start(ctx)
	}, func(ctx context.Context) error {
		if err := stop(ctx); err != nil {
			return err
		}
		if err := backend.Verify(ctx, previous); err != nil {
			return err
		}
		return backend.Finish(ctx)
	})
}

func repairStoppedService(ctx context.Context, record serviceRecord, digest [32]byte, retired func(uint32) (bool, error), start, stop func(context.Context) error) error {
	if ctx == nil || ctx.Err() != nil || record.Phase != serviceFailed || record.Digest != digest {
		return ErrRecovery
	}
	if _, err := encodeServiceRecord(record); err != nil {
		return ErrRecovery
	}
	exited, err := retired(record.PID)
	if err != nil || !exited {
		if err == nil {
			err = ErrRecovery
		}
		return installNativeError("service_repair_retired", ErrRecovery, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := start(ctx); err != nil {
		return installNativeError("service_repair_start", ErrRecovery, err)
	}
	// stop independently verifies the new process, clean receipt and kernel
	// process exit. A failed/unknown cleanup never authorizes replacement.
	if err := stop(ctx); err != nil {
		return installNativeError("service_repair_stop", ErrRecovery, err)
	}
	return nil
}

func retiredServiceProcess(pid uint32) (bool, error) {
	handle, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if err == windows.ERROR_INVALID_PARAMETER {
		return true, nil // The recorded PID no longer names a process.
	}
	if err != nil {
		return false, err
	}
	state, waitErr := windows.WaitForSingleObject(handle, 0)
	closeErr := windows.CloseHandle(handle)
	if waitErr != nil {
		return false, waitErr
	}
	if closeErr != nil {
		return false, closeErr
	}
	// PID reuse by any live process is conservatively refused.
	return state == windows.WAIT_OBJECT_0, nil
}
