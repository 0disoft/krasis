//go:build windows

package productpanel

import (
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/updatebundle"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackagedReceivedUpdateLaunchPreparation(t *testing.T) {
	source := os.Getenv("KRASIS_UPDATE_SMOKE")
	if source == "" {
		t.Skip("configured packaged check only")
	}
	archive, err := os.Open(filepath.Join(source, "krasis-windows-x64-smoke.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer archive.Close()
	info, err := archive.Stat()
	if err != nil {
		t.Fatal(err)
	}
	bundle, err := updatebundle.Read(archive, info.Size())
	if err != nil {
		t.Fatal(err)
	}
	appdata := t.TempDir()
	t.Setenv("LOCALAPPDATA", appdata)
	parent := filepath.Join(appdata, "Krasis", "updates")
	if err := os.MkdirAll(parent, 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := peercredentials.ProtectSecurityDirectory(parent); err != nil {
		t.Fatal(err)
	}
	root, err := os.OpenRoot(parent)
	if err != nil {
		t.Fatal(err)
	}
	defer root.Close()
	id := strings.Repeat("a", 32)
	if err := bundle.Stage(root, id); err != nil {
		t.Fatal(err)
	}
	called := false
	err = launchReceivedUpdate(filepath.Join(parent, id), bundle.Metadata().Revision, "A", bundle.Metadata().Generation-1, func(cmd *exec.Cmd) error {
		called = true
		if cmd.Path != filepath.Join(parent, id, "krasis-control.exe") {
			t.Fatal("wrong executable")
		}
		return nil
	})
	if err != nil || !called {
		t.Fatalf("launch preparation: %v, start=%v", err, called)
	}
	for _, tc := range []struct {
		revision   string
		generation int
	}{
		{strings.Repeat("0", 40), bundle.Metadata().Generation - 1},
		{bundle.Metadata().Revision, bundle.Metadata().Generation},
	} {
		err = launchReceivedUpdate(filepath.Join(parent, id), tc.revision, "A", tc.generation, func(*exec.Cmd) error { t.Fatal("unverified update reached launch"); return nil })
		if err == nil {
			t.Fatal("stale or mismatched update accepted")
		}
	}
}
