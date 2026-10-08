//go:build windows

package appinstall

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/launchrecord"
)

func registrationImage(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("registration fixture; never executed"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	return filepath.Join(dir, "krasis.exe")
}

func TestRepeatedRepairMigratesTaskBeforeLosingLaunchHistory(t *testing.T) {
	base, task := t.TempDir(), registrationImage(t)
	if err := launchrecord.Register(base, task, 234, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	// Model the native registrar's strict current/previous image ownership rule.
	recovery := func(base string, enabled bool) error {
		if !enabled {
			t.Fatal("enabled startup preference was lost")
		}
		r, err := launchrecord.Load(base)
		if err != nil {
			return err
		}
		allowed, err := launchrecord.RecoveryDirectories(base)
		if err != nil {
			return err
		}
		if !slices.Contains(allowed, filepath.Dir(task)) {
			return ErrInvalid
		}
		task = r.Executable
		return nil
	}
	startupCalls := 0
	startup := func(string, bool) error { startupCalls++; return nil }
	for i := range 3 {
		ui := registrationImage(t)
		if err := registerInstalledLaunch(base, ui, 235+i, strings.Repeat("b", 40), true, recovery, startup); err != nil {
			t.Fatal("repair lost ownership of existing task", err)
		}
		if task != ui {
			t.Fatal("task still points at an older image")
		}
	}
	if startupCalls != 3 {
		t.Fatal("startup preference was not registered for each repair")
	}
}

func TestFailedTaskPreflightPreservesLaunchHistoryAndDisabledPreference(t *testing.T) {
	base, old := t.TempDir(), registrationImage(t)
	if err := launchrecord.Register(base, old, 234, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	want := errors.New("foreign task")
	err := registerInstalledLaunch(base, registrationImage(t), 235, strings.Repeat("b", 40), false,
		func(_ string, enabled bool) error {
			if enabled {
				t.Fatal("disabled startup preference was enabled")
			}
			return want
		}, func(string, bool) error { t.Fatal("startup changed after failed preflight"); return nil })
	if !errors.Is(err, want) {
		t.Fatal(err)
	}
	if r, err := launchrecord.Load(base); err != nil || r.Executable != old {
		t.Fatal("failed task preflight replaced launch history", err)
	}
}

func TestCorruptImageRepairDisablesOwnedTaskBeforeRegisteringReplacement(t *testing.T) {
	base, old, next := t.TempDir(), registrationImage(t), registrationImage(t)
	if err := launchrecord.Register(base, old, 234, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(old, []byte("damaged installed image"), 0600); err != nil {
		t.Fatal(err)
	}
	var states []bool
	recovery := func(base string, enabled bool) error {
		states = append(states, enabled)
		r, err := launchrecord.Load(base)
		if err != nil {
			return err
		}
		if len(states) == 1 && r.Executable != old {
			t.Fatal("lost task ownership history before disabling it")
		}
		if enabled {
			_, err = launchrecord.Inspect(base)
		}
		return err
	}
	if err := registerInstalledLaunch(base, next, 238, strings.Repeat("b", 40), true, recovery,
		func(_ string, enabled bool) error {
			if !enabled {
				t.Fatal("repair lost the user's startup preference")
			}
			return nil
		}); err != nil {
		t.Fatal("corrupt image prevented repair", err)
	}
	if !slices.Equal(states, []bool{false, true}) {
		t.Fatal("damaged image task was re-armed", states)
	}
}
