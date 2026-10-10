//go:build windows

package inputworker

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"io"
	"path/filepath"
	"time"

	"golang.org/x/sys/windows"
)

func runInstalledService(ctx context.Context, installation *Installation, digest [sha256.Size]byte) error {
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return ErrAuthority
	}
	record := serviceRecord{PID: windows.GetCurrentProcessId(), Digest: digest}
	if _, err := rand.Read(record.Run[:]); err != nil {
		return ErrAuthority
	}
	root = filepath.Join(root, "Krasis")
	persist := func(record serviceRecord) error {
		// Service stop has already cancelled ctx; receipt persistence must not
		// inherit that cancellation or claim a clean exit without recording it.
		writeCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		return writeServiceRecord(writeCtx, root, record)
	}
	return runRecordedService(ctx, record, persist, func(ctx context.Context, ready func() error) error {
		return runInstalledLoginService(ctx, installation, ready)
	})
}

func writeServiceRecord(ctx context.Context, root string, record serviceRecord) error {
	pinned, err := openProtected(root, true)
	if err != nil {
		return err
	}
	defer pinned.Close()
	data, err := encodeServiceRecord(record)
	if err != nil {
		return err
	}
	pending := filepath.Join(root, "INPUT-SERVICE.pending")
	if err := writeInstallFile(ctx, pending, data[:]); err != nil {
		return err
	}
	destination := filepath.Join(root, "INPUT-SERVICE.record")
	if !missingInstallPath(destination) {
		previous, err := openProtected(destination, false)
		if err != nil {
			return err
		}
		if previous.Close() != nil {
			return ErrAuthority
		}
	}
	if ctx.Err() != nil {
		return ErrAuthority
	}
	return moveInstallPath(pending, destination, true)
}

func readServiceRecord(root string) (serviceRecord, error) {
	pinned, err := openProtected(root, true)
	if err != nil {
		return serviceRecord{}, err
	}
	defer pinned.Close()
	file, err := openProtected(filepath.Join(root, "INPUT-SERVICE.record"), false)
	if err != nil {
		return serviceRecord{}, err
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, serviceRecordSize+1))
	if err != nil {
		return serviceRecord{}, ErrAuthority
	}
	return decodeServiceRecord(data)
}
