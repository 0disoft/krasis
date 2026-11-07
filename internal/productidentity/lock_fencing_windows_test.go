//go:build windows

package productidentity

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestActiveIdentityLockCannotBeDeletedOrReplaced(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(map[bool]string{false: "exclusive", true: "shared"}[shared], func(t *testing.T) {
			store := testStore(t)
			if _, _, err := store.OpenOrCreate(time.Now()); err != nil {
				t.Fatal(err)
			}
			var release func()
			var err error
			if shared {
				_, release, err = store.openTransportReadLocked()
			} else {
				_, release, err = store.openLocked(false)
			}
			if err != nil {
				t.Fatal(err)
			}
			defer release()
			path := filepath.Join(store.directory, lockFile)
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
			if _, err := store.Rotate(time.Now()); !errors.Is(err, ErrStoreBusy) {
				t.Fatalf("competing writer: %v", err)
			}
		})
	}
}
