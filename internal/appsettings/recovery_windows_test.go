//go:build windows

package appsettings

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"github.com/0disoft/krasis/internal/launchrecord"
	"golang.org/x/sys/windows"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestRecoveryTaskRegistrationIsUserScopedAndReversible(t *testing.T) {
	t.Setenv("KRASIS_RECOVERY_DEBUG", "1")
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	suffix := "-test-" + hex.EncodeToString(nonce[:])
	base := t.TempDir()
	bundle := t.TempDir()
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte("not executable; task removed before trigger"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := launchrecord.Register(base, filepath.Join(bundle, "krasis.exe"), 192, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := setRecoveryTask(base, suffix, false); err != nil {
			t.Error("temporary task cleanup failed", err)
		}
	})
	// Migrate the previously shipped console action under the isolated test name.
	system, err := windows.GetSystemDirectory()
	if err != nil {
		t.Fatal(err)
	}
	legacy := strings.ReplaceAll(recoveryTaskScript, "Krasis recovery v2", "Krasis recovery v1")
	legacy = strings.ReplaceAll(legacy, RecoveryArgument, "app ensure --json")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	command := recoveryScriptCommand(ctx, system, legacy)
	cli := filepath.Join(base, "Krasis", "launch", "krasis-cli.exe")
	command.Env = append(os.Environ(), "KRASIS_RECOVERY_TARGET="+cli, "KRASIS_RECOVERY_LEGACY_TARGET="+cli, "KRASIS_RECOVERY_PREVIOUS=[]", "KRASIS_RECOVERY_SUFFIX="+suffix, "KRASIS_RECOVERY_ENABLED=1", "KRASIS_RECOVERY_CHECK=0")
	if out, err := command.CombinedOutput(); err != nil {
		t.Fatalf("legacy fixture: %v %s", err, out)
	}
	if err := runRecoveryTask(base, suffix, false, true); !errors.Is(err, errRecoveryTaskInactive) {
		t.Fatal("console action considered migrated", err)
	}
	for range 2 {
		if err := setRecoveryTask(base, suffix, true); err != nil {
			t.Fatal(err)
		}
	}
	if err := runRecoveryTask(base, suffix, false, true); err != nil {
		t.Fatal("read-only query did not find registered task", err)
	}
	// Registration can rotate twice before the next successful scheduler update:
	// first the portable CLI records a build folder, then the UI publishes its
	// immutable copy. Keep the actual old action owned across both rotations.
	for range 2 {
		relocated := t.TempDir()
		for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
			if err := os.WriteFile(filepath.Join(relocated, name), []byte("not executable; isolated task"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		if err := launchrecord.Register(base, filepath.Join(relocated, "krasis.exe"), 192, strings.Repeat("a", 40)); err != nil {
			t.Fatal(err)
		}
	}
	if err := runRecoveryTask(base, suffix, false, true); !errors.Is(err, errRecoveryTaskInactive) {
		t.Fatal("previous task became foreign or current after double rotation", err)
	}
	if err := setRecoveryTask(base, suffix, true); err != nil {
		t.Fatal("double-rotated owned task could not migrate", err)
	}
	// A new successful UI folder must update the same owned task, rather than
	// pinning it forever to an old generation or replacing an unrelated task.
	next := t.TempDir()
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(next, name), []byte("not executable; task removed before trigger"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := launchrecord.Register(base, filepath.Join(next, "krasis.exe"), 193, strings.Repeat("b", 40)); err != nil {
		t.Fatal(err)
	}
	if err := runRecoveryTask(base, suffix, false, true); !errors.Is(err, errRecoveryTaskInactive) {
		t.Fatal("stale action considered current", err)
	}
	if err := setRecoveryTask(base, suffix, true); err != nil {
		t.Fatal("owned previous action not migrated", err)
	}
	if err := runRecoveryTask(base, suffix, false, true); err != nil {
		t.Fatal("updated action not verified", err)
	}
	if err := setRecoveryTask(base, suffix, false); err != nil {
		t.Fatal(err)
	}
	if err := runRecoveryTask(base, suffix, false, true); !errors.Is(err, errRecoveryTaskInactive) {
		t.Fatal("absent recovery task was reported as active", err)
	}
	if err := setRecoveryTask(base, suffix, false); err != nil {
		t.Fatal("absence not idempotent", err)
	}
}

func TestRecoveryTaskPreflightWithoutHistoryAllowsAbsenceButRejectsOrphan(t *testing.T) {
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		t.Fatal(err)
	}
	suffix := "-test-" + hex.EncodeToString(nonce[:])
	base := t.TempDir()
	// First install has neither launch history nor a task. Inspection must
	// still query the scheduler, without publishing a synthetic launch record.
	if err := runRecoveryTask(base, suffix, false, true); !errors.Is(err, errRecoveryTaskInactive) {
		t.Fatal("first-install preflight rejected absent task", err)
	}
	bundle := t.TempDir()
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(bundle, name), []byte("not executable; isolated task removed before trigger"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := launchrecord.Register(base, filepath.Join(bundle, "krasis.exe"), 252, strings.Repeat("a", 40)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := setRecoveryTask(base, suffix, false); err != nil {
			t.Error("temporary task cleanup failed", err)
		}
	})
	if err := setRecoveryTask(base, suffix, true); err != nil {
		t.Fatal(err)
	}
	current := filepath.Join(base, "Krasis", "launch", "current.json")
	backup := current + ".test-backup"
	if err := os.Rename(current, backup); err != nil {
		t.Fatal(err)
	}
	// The durable scheduler receipt is also ownership proof. A missing UI record
	// alone no longer makes this an orphan, but can never report it current.
	ownedErr := runRecoveryTask(base, suffix, false, true)
	receipt := filepath.Join(base, "Krasis", "launch", "recovery-task.json")
	if err := os.Rename(receipt, receipt+".test-backup"); err != nil {
		_ = os.Rename(backup, current)
		t.Fatal(err)
	}
	err := runRecoveryTask(base, suffix, false, true)
	// Restore history even when the assertion fails so cleanup retains proof.
	if restoreErr := os.Rename(receipt+".test-backup", receipt); restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if restoreErr := os.Rename(backup, current); restoreErr != nil {
		t.Fatal(restoreErr)
	}
	if !errors.Is(ownedErr, errRecoveryTaskInactive) {
		t.Fatal("recorded old task became current or foreign without current UI record", ownedErr)
	}
	if err == nil || errors.Is(err, errRecoveryTaskInactive) {
		t.Fatal("orphan task accepted without launch ownership", err)
	}
	if err := runRecoveryTask(base, suffix, false, true); err != nil {
		t.Fatal("failed preflight changed the previously owned task", err)
	}
}
