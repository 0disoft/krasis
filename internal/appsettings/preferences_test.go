package appsettings

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

func TestPreferencesLeaseOwnsWritesUntilClosed(t *testing.T) {
	base := t.TempDir()
	initial := Settings{Schema: Schema, StartAtLogin: true, AutoUpdate: true}
	if err := Save(base, initial); err != nil {
		t.Fatal(err)
	}
	lease, err := LockPreferences(base)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	if err := Save(base, Settings{Schema: Schema}); err == nil {
		t.Fatal("another writer bypassed the preference transaction")
	}
	if _, err := Initialize(base); err == nil {
		t.Fatal("initialization bypassed the preference transaction")
	}
	if value, err := Load(base); err != nil || value != initial {
		t.Fatal("lock blocked reads or a rejected writer changed settings", value, err)
	}
	if value, err := lease.Initialize(); err != nil || value != initial {
		t.Fatal("owner could not initialize under its lease", value, err)
	}
	if err := Save(t.TempDir(), initial); err != nil {
		t.Fatal("another profile was blocked", err)
	}
	disabled := Settings{Schema: Schema}
	if err := lease.Save(disabled); err != nil {
		t.Fatal("owner could not save under its lease", err)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if err := lease.Save(initial); !errors.Is(err, os.ErrClosed) {
		t.Fatal("closed owner wrote settings", err)
	}
	if _, err := lease.Initialize(); !errors.Is(err, os.ErrClosed) {
		t.Fatal("closed owner initialized settings", err)
	}
	if value, err := Load(base); err != nil || value != disabled {
		t.Fatal(value, err)
	}
	if err := Save(base, initial); err != nil {
		t.Fatal("released lock still fenced writers", err)
	}
}

func TestPreferencesLeaseExcludesOtherProcess(t *testing.T) {
	base := t.TempDir()
	lease, err := LockPreferences(base)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	probe := func(want string) {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		cmd := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestPreferencesLeaseProcessHelper$")
		cmd.Env = append(os.Environ(), "KRASIS_PREFERENCES_TEST_BASE="+base)
		output, err := cmd.CombinedOutput()
		if err != nil || !strings.Contains(string(output), "preferences="+want) {
			t.Fatalf("cross-process lock: want %s, got %s (%v)", want, output, err)
		}
	}
	probe("busy")
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	probe("saved")
}

func TestPreferencesLeaseProcessHelper(t *testing.T) {
	base := os.Getenv("KRASIS_PREFERENCES_TEST_BASE")
	if base == "" {
		t.Skip("subprocess helper")
	}
	if err := Save(base, Settings{Schema: Schema}); err != nil {
		fmt.Println("preferences=busy")
		return
	}
	fmt.Println("preferences=saved")
}
