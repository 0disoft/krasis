//go:build windows

package appinstall

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/productidentity"
)

func TestInstallHistoryPreflightRejectsBadRecordsButAllowsImageRepair(t *testing.T) {
	base, ui := t.TempDir(), registrationImage(t)
	if err := validateInstallHistory(base, 241); err != nil {
		t.Fatal("first install rejected", err)
	}
	if err := launchrecord.Register(base, ui, 241, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	if err := validateInstallHistory(base, 240); !errors.Is(err, ErrOlder) {
		t.Fatal("downgrade accepted", err)
	}
	if err := os.WriteFile(ui, []byte("damaged image; still repairable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := validateInstallHistory(base, 241); err != nil {
		t.Fatal("image repair blocked", err)
	}
	dir, err := productidentity.PrepareLaunch(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	for _, name := range []string{"previous.json", "current.json"} {
		if err := replace(dir, name, []byte("{}")); err != nil {
			t.Fatal(err)
		}
		if err := validateInstallHistory(base, 241); err == nil {
			t.Fatal("corrupt history accepted", name)
		}
		if err := dir.Remove(name); err != nil {
			t.Fatal(err)
		}
	}
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(paths.Root, "app")); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("history preflight created an app installation", err)
	}
}
