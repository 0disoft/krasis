//go:build windows

package appinstall

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/peercredentials"
)

func TestInstallDirectoryCanStageWhileProtectedHandleIsOpen(t *testing.T) {
	base := t.TempDir()
	for _, phase := range []string{"first_install", "repair"} {
		t.Run(phase, func(t *testing.T) {
			dir, path, err := directory(base)
			if err != nil {
				t.Fatal(err)
			}
			defer dir.Close()
			if _, err := peercredentials.ValidateSecurityDirectory(path); err != nil {
				t.Fatal("installation root lost its protected ACL", err)
			}
			root, err := os.OpenRoot(path)
			if err != nil {
				t.Fatal("staging conflicts with setup's own directory handle", err)
			}
			defer root.Close()
			if err := root.Mkdir(phase, 0700); err != nil {
				t.Fatal(err)
			}
			// Receipt writes must still work while the staging root is held.
			if err := replace(dir, "installation.json", []byte(phase)); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(path, phase)); err != nil {
				t.Fatal(err)
			}
		})
	}
}
