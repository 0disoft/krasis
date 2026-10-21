//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sys/windows"
)

func openObservationTestWriter(path string) (windows.Handle, error) {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return 0, err
	}
	return windows.CreateFile(name, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_FLAG_OPEN_REPARSE_POINT, 0)
}

func TestInstallationObservationExcludesWriterUntilAllImagePinsClose(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "INPUT-TRANSACTION.lock")
	if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	installation, err := observeInstallation(root, func() (*Installation, error) {
		handle, err := openObservationTestWriter(path)
		if err == nil {
			_ = windows.CloseHandle(handle)
			t.Error("installer entered between lock observation and image pinning")
		} else if !errors.Is(err, windows.ERROR_SHARING_VIOLATION) {
			t.Error(err)
		}
		imagePath := filepath.Join(root, "image.exe")
		data := []byte("verified image fixture")
		if err := os.WriteFile(imagePath, data, 0600); err != nil {
			return nil, err
		}
		image, err := lockImage(imagePath, sha256.Sum256(data))
		if err != nil {
			return nil, err
		}
		return &Installation{files: []*os.File{image}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	defer installation.Close()
	handle, err := openObservationTestWriter(path)
	if err == nil {
		_ = windows.CloseHandle(handle)
		t.Error("installer overlapped live observation")
	}
	if installation.Close() != nil {
		t.Fatal("observation close failed")
	}
	handle, err = openObservationTestWriter(path)
	if err != nil {
		t.Fatal("observation leaked its lock", err)
	}
	_ = windows.CloseHandle(handle)
	if err := os.WriteFile(filepath.Join(root, "image.exe"), []byte("replacement"), 0600); err != nil {
		t.Fatal("observer released transaction lock before image pin", err)
	}
}

func TestInstallationObservationRejectsWriterAndReleasesFailedLoad(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "INPUT-TRANSACTION.lock")
	load := func() (*Installation, error) {
		t.Error("busy/missing lock reached image loader")
		return &Installation{}, nil
	}
	if _, err := observeInstallation(root, load); err == nil {
		t.Fatal("missing lock admitted observation")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("observer created transaction lock")
	}
	if err := os.WriteFile(path, nil, 0600); err != nil {
		t.Fatal(err)
	}
	handle, err := openObservationTestWriter(path)
	if err != nil {
		t.Fatal(err)
	}
	_, busyErr := observeInstallation(root, load)
	_ = windows.CloseHandle(handle)
	if busyErr == nil {
		t.Fatal("observation entered running installer")
	}
	if _, err := observeInstallation(root, func() (*Installation, error) { return nil, ErrAuthority }); err == nil {
		t.Fatal("failed installation load accepted")
	}
	handle, err = openObservationTestWriter(path)
	if err != nil {
		t.Fatal("failed observation retained its transaction lock", err)
	}
	_ = windows.CloseHandle(handle)
}

func TestInstallerWaitsOnlyForReadersWithinOneBudget(t *testing.T) {
	for _, mode := range []string{"reader_returns", "reader_stalls", "caller_expires", "other_installer"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				root := t.TempDir()
				path := filepath.Join(root, "INPUT-TRANSACTION.lock")
				if err := os.WriteFile(path, nil, 0600); err != nil {
					t.Fatal(err)
				}
				name, _ := windows.UTF16PtrFromString(path)
				access, sharing := uint32(windows.GENERIC_READ), uint32(windows.FILE_SHARE_READ)
				if mode == "other_installer" {
					access, sharing = windows.GENERIC_READ|windows.GENERIC_WRITE, 0
				}
				held, err := windows.CreateFile(name, access, sharing, nil, windows.OPEN_EXISTING, 0, 0)
				if err != nil {
					t.Fatal(err)
				}
				defer func() {
					if held != 0 {
						_ = windows.CloseHandle(held)
					}
				}()
				if mode == "reader_returns" {
					time.AfterFunc(40*time.Millisecond, func() { _ = windows.CloseHandle(held); held = 0 })
				}
				ctx := context.Background()
				if mode == "caller_expires" {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, 50*time.Millisecond)
					defer cancel()
				}
				started := time.Now()
				file, err := waitInstallWrite(ctx, path, func(path string) (*os.File, error) {
					h, err := openObservationTestWriter(path)
					if err != nil {
						return nil, err
					}
					return os.NewFile(uintptr(h), "fixture-writer"), nil
				})
				synctest.Wait() // Join any reader-return callback before examining held.
				if mode == "reader_returns" {
					if err != nil {
						t.Fatal("reader retirement did not admit installer", err)
					}
					_ = file.Close()
				} else if mode == "reader_stalls" || mode == "caller_expires" {
					budget := 2 * time.Second
					if mode == "caller_expires" {
						budget = 50 * time.Millisecond
					}
					if file != nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != budget {
						t.Fatal("reader defeated installer admission budget", err, time.Since(started))
					}
				} else if file != nil || !errors.Is(err, windows.ERROR_SHARING_VIOLATION) || time.Since(started) != 0 {
					t.Fatal("installer queued behind another transaction", err, time.Since(started))
				}
			})
		})
	}
}
