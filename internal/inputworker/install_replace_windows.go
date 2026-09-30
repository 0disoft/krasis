//go:build windows

package inputworker

import (
	"context"
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/windows"
)

func (files *installFiles) replace(ctx context.Context, target installImages) error {
	if err := files.check(ctx); err != nil {
		return err
	}
	if !missingInstallPath(files.directory(installRetired)) {
		return ErrAuthority
	}
	if target.valid() {
		candidate, err := files.readImages(ctx, installCandidate, &target)
		if err != nil {
			return err
		}
		if candidate.Close() != nil {
			return ErrAuthority
		}
	} else if target != (installImages{}) {
		return ErrAuthority
	}
	if !missingInstallPath(files.directory(installActive)) {
		old, err := files.readImages(ctx, installActive, nil)
		if err != nil {
			return err
		}
		if old.Close() != nil {
			return ErrAuthority
		}
		if moveInstallPath(files.directory(installActive), files.directory(installRetired), false) != nil {
			return ErrAuthority
		}
	}
	if ctx.Err() != nil {
		return ErrAuthority
	}
	if target.valid() {
		return moveInstallPath(files.directory(installCandidate), files.directory(installActive), false)
	}
	return nil
}

func (files *installFiles) restore(ctx context.Context, previous installSnapshot) error {
	if err := files.check(ctx); err != nil || !previous.valid() {
		return ErrAuthority
	}
	// If replacement failed because a running agent pinned its old image, that
	// installation is still intact. Do not delete it just to copy it back.
	if previous.Present {
		if current, err := files.readImages(ctx, installActive, &previous.Images); err == nil {
			return current.Close()
		}
		backup, err := files.readImages(ctx, installBackup, &previous.Images)
		if err != nil {
			return err
		}
		if backup.Close() != nil {
			return ErrAuthority
		}
	}
	if err := files.removeDirectory(ctx, installActive); err != nil {
		return err
	}
	if previous.Present {
		return moveInstallPath(files.directory(installBackup), files.directory(installActive), false)
	}
	return nil
}

// No recursive deletion and no request paths. Unexpected names, subdirectories,
// reparse points and unprotected files leave the recovery journal intact.
func (files *installFiles) removeDirectory(ctx context.Context, kind installDirectory) error {
	if err := files.check(ctx); err != nil {
		return err
	}
	directory := files.directory(kind)
	if missingInstallPath(directory) {
		return nil
	}
	pinned, err := openProtected(directory, true)
	if err != nil {
		return err
	}
	defer pinned.Close()
	reader, err := os.Open(directory)
	if err != nil {
		return ErrAuthority
	}
	entries, readErr := reader.ReadDir(len(installFileNames) + 1)
	_ = reader.Close()
	if readErr != nil && readErr != io.EOF || len(entries) > len(installFileNames) {
		return ErrAuthority
	}
	for _, entry := range entries {
		allowed := false
		for _, name := range installFileNames {
			allowed = allowed || entry.Name() == name
		}
		if !allowed || entry.IsDir() || entry.Type()&os.ModeType != 0 {
			return ErrAuthority
		}
		// Validate the entire list before removing its first member.
		file, err := openProtected(filepath.Join(directory, entry.Name()), false)
		if err != nil {
			return err
		}
		_ = file.Close()
	}
	for _, entry := range entries {
		if err := removeInstallFile(ctx, filepath.Join(directory, entry.Name())); err != nil {
			return err
		}
	}
	if ctx.Err() != nil || pinned.Close() != nil {
		return ErrAuthority
	}
	name, err := windows.UTF16PtrFromString(directory)
	if err != nil || windows.RemoveDirectory(name) != nil {
		return ErrAuthority
	}
	return nil
}

func removeInstallFile(ctx context.Context, path string) error {
	if ctx.Err() != nil {
		return ErrAuthority
	}
	if missingInstallPath(path) {
		return nil
	}
	file, err := openProtected(path, false)
	if err != nil {
		return err
	}
	if file.Close() != nil {
		return ErrAuthority
	}
	name, err := windows.UTF16PtrFromString(path)
	if err != nil || windows.DeleteFile(name) != nil {
		return ErrAuthority
	}
	return nil
}

func (files *installFiles) finish(ctx context.Context) error {
	if err := files.check(ctx); err != nil {
		return err
	}
	for _, kind := range []installDirectory{installCandidate, installBackup, installRetired} {
		if err := files.removeDirectory(ctx, kind); err != nil {
			return err
		}
	}
	for _, name := range []string{"INPUT-TRANSACTION.pending", "INPUT-TRANSACTION.json"} {
		if err := removeInstallFile(ctx, filepath.Join(files.root, name)); err != nil {
			return err
		}
	}
	return nil
}
