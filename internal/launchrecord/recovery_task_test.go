package launchrecord

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func recoveryBundle(t *testing.T, base string, generation int) string {
	t.Helper()
	dir := t.TempDir()
	for _, name := range []string{"krasis.exe", "krasis-cli.exe"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("MZ-fixture"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	if err := Register(base, filepath.Join(dir, "krasis.exe"), generation, "fixture"); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestRecoveryTaskTargetSurvivesTwoLaunchRotationsAndFailedMigration(t *testing.T) {
	base := t.TempDir()
	old := recoveryBundle(t, base, 293)
	if err := UpdateRecoveryTask(base, true, func(Record, []string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	recoveryBundle(t, base, 294)           // CLI app repair in the build folder.
	latest := recoveryBundle(t, base, 294) // UI promotes its immutable update copy.
	paths, err := RecoveryDirectories(base)
	if err != nil || len(paths) != 3 || paths[0] != latest || paths[2] != old {
		t.Fatal("live task target forgotten after launch rotation", paths, err)
	}
	failure := errors.New("scheduler refused migration")
	if err := UpdateRecoveryTask(base, true, func(Record, []string) error { return failure }); err != failure {
		t.Fatal(err)
	}
	pinned, err := loadNamed(base, recoveryTaskRecord)
	if err != nil || filepath.Dir(pinned.Executable) != old {
		t.Fatal("failed task update replaced ownership proof", pinned, err)
	}
	if err := UpdateRecoveryTask(base, true, func(record Record, known []string) error {
		if filepath.Dir(record.Executable) != latest || len(known) != 3 || known[2] != old {
			t.Fatal("task migration lost exact ownership", record, known)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	paths, err = RecoveryDirectories(base)
	if err != nil || len(paths) != 2 {
		t.Fatal("retired task directory retained", paths, err)
	}
	if err := UpdateRecoveryTask(base, false, func(Record, []string) error { return nil }); err != nil {
		t.Fatal(err)
	}
	if _, err := loadNamed(base, recoveryTaskRecord); !errors.Is(err, ErrMissing) {
		t.Fatal("disabled task retained its record", err)
	}
}

func TestUnverifiedTaskHistoryBlocksMutationAndPruning(t *testing.T) {
	base := t.TempDir()
	recoveryBundle(t, base, 294)
	dir, _ := Directory(base)
	if err := os.WriteFile(filepath.Join(dir, recoveryTaskRecord), []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := RecoveryDirectories(base); err == nil {
		t.Fatal("corrupt task receipt allowed pruning")
	}
	if err := UpdateRecoveryTask(base, false, func(Record, []string) error { t.Fatal("unknown ownership mutated scheduler"); return nil }); err == nil {
		t.Fatal("corrupt ownership record ignored")
	}
}
