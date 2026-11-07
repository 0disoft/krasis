//go:build windows

package truststore

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActiveTrustLockCannotBeDeletedOrReplaced(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "exclusive", true: "shared"}[shared], func(t *testing.T) {
			store := mustStore(t)
			if _, err := store.Import(testPeer(), "peer", true, time.Now()); err != nil {
				t.Fatal(err)
			}
			var release func()
			var err error
			if shared {
				_, _, release, err = store.openReadLocked()
			} else {
				_, _, release, err = store.openLocked(false)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			path := filepath.Join(store.directory, lockFilename)
			if err := os.Remove(path); err == nil {
				t.Fatal("active lock was deleted")
			}
			if err := os.Rename(path, path+".moved"); err == nil {
				t.Fatal("active lock was renamed")
			}
			replacement := filepath.Join(store.directory, "replacement")
			if err := os.WriteFile(replacement, []byte("replacement"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := os.Rename(replacement, path); err == nil {
				t.Fatal("active lock was replaced")
			}
			if _, err := store.Update(testPeer().CertificateSHA256, "peer", true, time.Now()); !errors.Is(err, ErrStoreBusy) {
				t.Fatalf("competing writer: %v", err)
			}
		})
	}
}
