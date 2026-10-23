//go:build windows

package truststore

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/securefs"
)

func TestStoreReadOpenTraceRetainsNativeFailure(t *testing.T) {
	directory, err := securefs.OpenReadDirectory(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	defer directory.Close()
	trace := NewReadTrace()
	trace.enter("store_read")
	_, err = readVerified(directory, trace, func(*os.File) error {
		t.Fatal("missing file reached ACL validation")
		return nil
	})
	snapshot := trace.Snapshot()
	_, enclosing := snapshot.Durations["store_read_open"]
	_, independent := snapshot.Durations["store_read_open_native"]
	if !errors.Is(err, ErrStoreNotFound) || snapshot.Phase != "store_read_open_native" || !enclosing || independent {
		t.Fatal("native failure lost its active call or enclosing interval", snapshot, err)
	}
}

func TestStoreReadWorksWithReadOnlySharing(t *testing.T) {
	for _, target := range []string{"directory", "lock"} {
		t.Run(target, func(t *testing.T) {
			store := mustStore(t)
			if _, err := store.Import(testPeer(), "peer", true, time.Now()); err != nil {
				t.Fatal(err)
			}
			path := store.directory
			flags := uint32(syscall.FILE_FLAG_BACKUP_SEMANTICS)
			sharing := uint32(syscall.FILE_SHARE_READ | syscall.FILE_SHARE_WRITE)
			if target == "lock" {
				path = filepath.Join(path, lockFilename)
				flags = syscall.FILE_ATTRIBUTE_NORMAL
				sharing = syscall.FILE_SHARE_READ
			}
			name, err := syscall.UTF16PtrFromString(path)
			if err != nil {
				t.Fatal(err)
			}
			handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, sharing, nil, syscall.OPEN_EXISTING, flags, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer syscall.CloseHandle(handle)
			document, err := store.List()
			if err != nil || len(document.Peers) != 1 || document.Peers[0].Alias != "peer" {
				t.Fatal("read-only trust observation failed", err)
			}
		})
	}
}
