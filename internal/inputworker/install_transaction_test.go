package inputworker

import (
	"context"
	"crypto/sha256"
	"errors"
	"strings"
	"testing"
)

var installTestFailure = errors.New("native error must not escape")

type installTestBackend struct {
	journal       *installJournal
	state         installSnapshot
	backup        installSnapshot
	staged        bool
	locked        bool
	fail          string
	failOnce      bool
	saveFail      installPhase
	savePersist   bool
	cancelAt      string
	cancel        context.CancelFunc
	operations    []string
	recoveryCtxOK bool
}

func installTestImages(label string) installImages {
	return installImages{Agent: sha256.Sum256([]byte(label + "agent")), Service: sha256.Sum256([]byte(label + "service"))}
}

func (backend *installTestBackend) operation(ctx context.Context, name string) error {
	backend.operations = append(backend.operations, name)
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if backend.cancelAt == name {
		backend.cancel()
		backend.cancelAt = ""
	}
	if backend.fail == name {
		if backend.failOnce {
			backend.fail = ""
		}
		return installTestFailure
	}
	return nil
}

func (backend *installTestBackend) Lock(ctx context.Context) (func(), error) {
	if backend.locked || backend.operation(ctx, "lock") != nil {
		return nil, installTestFailure
	}
	backend.locked = true
	return func() { backend.locked = false }, nil
}

func (backend *installTestBackend) Load(ctx context.Context) (*installJournal, error) {
	if err := backend.operation(ctx, "load"); err != nil {
		return nil, err
	}
	if backend.journal == nil {
		return nil, nil
	}
	copy := *backend.journal
	return &copy, nil
}

func (backend *installTestBackend) Snapshot(ctx context.Context) (installSnapshot, error) {
	return backend.state, backend.operation(ctx, "snapshot")
}

func (backend *installTestBackend) Save(ctx context.Context, journal installJournal) error {
	if err := backend.operation(ctx, "save:"+string(journal.Phase)); err != nil {
		return err
	}
	if backend.saveFail != journal.Phase || backend.savePersist {
		backend.journal = &journal
	}
	if backend.saveFail == journal.Phase {
		return installTestFailure
	}
	return nil
}

func (backend *installTestBackend) Stage(ctx context.Context, journal installJournal) error {
	if err := backend.operation(ctx, "stage"); err != nil {
		return err
	}
	backend.backup = journal.Previous
	backend.staged = true
	return nil
}

func (backend *installTestBackend) Stop(ctx context.Context) error {
	if err := backend.operation(ctx, "stop"); err != nil {
		return err
	}
	backend.state.Running = false
	return nil
}

func (backend *installTestBackend) Replace(ctx context.Context, images installImages) error {
	if !backend.staged || backend.state.Running {
		return installTestFailure
	}
	if err := backend.operation(ctx, "replace"); err != nil {
		return err
	}
	backend.state = installSnapshot{Present: images.valid(), Images: images}
	return nil
}

func (backend *installTestBackend) Start(ctx context.Context) error {
	if err := backend.operation(ctx, "start"); err != nil {
		// Model an SCM call which started a process before readiness failed.
		if ctx.Err() == nil && backend.state.Present {
			backend.state.Running = true
		}
		return err
	}
	backend.state.Running = true
	return nil
}

func (backend *installTestBackend) Verify(ctx context.Context, expected installSnapshot) error {
	if err := backend.operation(ctx, "verify"); err != nil {
		return err
	}
	if backend.state != expected {
		return installTestFailure
	}
	return nil
}

func (backend *installTestBackend) Restore(ctx context.Context, expected installSnapshot) error {
	if err := backend.operation(ctx, "restore"); err != nil {
		return err
	}
	if !backend.staged || backend.state.Running || expected != backend.backup {
		return installTestFailure
	}
	backend.recoveryCtxOK = ctx.Err() == nil
	backend.state = expected
	backend.state.Running = false
	return nil
}

func (backend *installTestBackend) Finish(ctx context.Context) error {
	if err := backend.operation(ctx, "finish"); err != nil {
		return err
	}
	backend.staged = false
	backend.journal = nil
	return nil
}

func TestInstallTransactionApplyAndRemove(t *testing.T) {
	old := installSnapshot{Present: true, Running: true, Images: installTestImages("old")}
	for _, example := range []struct {
		name     string
		previous installSnapshot
		target   installImages
	}{
		{"install", installSnapshot{}, installTestImages("new")},
		{"update", old, installTestImages("new")},
		{"remove", old, installImages{}},
		{"remove_absent", installSnapshot{}, installImages{}},
	} {
		t.Run(example.name, func(t *testing.T) {
			backend := &installTestBackend{state: example.previous}
			if err := applyInputInstallation(context.Background(), backend, example.target); err != nil {
				t.Fatal(err)
			}
			want := installSnapshot{Present: example.target.valid(), Running: example.target.valid(), Images: example.target}
			if backend.state != want || backend.locked || backend.journal != nil || backend.staged {
				t.Fatal("transaction did not complete in requested state")
			}
		})
	}
}

func TestInstallTransactionFailedReplacementRestoresPriorRunningState(t *testing.T) {
	for _, running := range []bool{false, true} {
		for _, present := range []bool{false, true} {
			if running && !present {
				continue
			}
			for _, fail := range []string{"stage", "replace"} {
				old := installSnapshot{Present: present, Running: running}
				if present {
					old.Images = installTestImages("old")
				}
				backend := &installTestBackend{state: old, fail: fail}
				if err := applyInputInstallation(context.Background(), backend, installTestImages("new")); err != errInstallFailed {
					t.Fatalf("failure %s: %v", fail, err)
				}
				if backend.state != old || backend.journal != nil || backend.locked {
					t.Fatal("rollback lost prior installation/running state")
				}
			}
		}
	}
}

func TestInstallTransactionUnknownJournalWriteStopsEffectsAndReconciles(t *testing.T) {
	for _, persist := range []bool{false, true} {
		for _, phase := range []installPhase{installStage, installStop, installReplace, installStart, installCommit} {
			old := installSnapshot{Present: true, Running: true, Images: installTestImages("old")}
			target := installTestImages("new")
			backend := &installTestBackend{state: old, saveFail: phase, savePersist: persist}
			if err := applyInputInstallation(context.Background(), backend, target); err != errInstallJournal {
				t.Fatalf("phase %s: journal ambiguity not reported: %v", phase, err)
			}
			if last := backend.operations[len(backend.operations)-1]; last != "save:"+string(phase) {
				t.Fatal("effects continued after unknown journal write")
			}
			backend.saveFail = ""
			if err := recoverInputInstallation(backend); err != nil {
				t.Fatalf("phase %s: durable reconciliation failed: %v", phase, err)
			}
			want := old
			if persist && phase == installCommit {
				want = installSnapshot{Present: true, Running: true, Images: target}
			}
			if backend.state != want || backend.journal != nil {
				t.Fatal("recovery guessed the wrong committed state")
			}
		}
	}
}

func TestInstallTransactionCrashRecoveryAtEveryPhase(t *testing.T) {
	old := installSnapshot{Present: true, Running: true, Images: installTestImages("old")}
	target := installTestImages("new")
	for _, phase := range []installPhase{installStage, installStop, installReplace, installStart, installCommit,
		installRollbackStop, installRollbackRestore, installRollbackStart, installRollbackCommit} {
		t.Run(string(phase), func(t *testing.T) {
			journal := &installJournal{Schema: "krasis.input.transaction/v1", Phase: phase, Previous: old, Target: target}
			backend := &installTestBackend{state: old, journal: journal, backup: old, staged: true}
			switch phase {
			case installStop, installRollbackStart:
				backend.state.Running = false
			case installReplace, installRollbackRestore:
				backend.state = installSnapshot{Present: true, Images: target}
			case installStart, installRollbackStop, installCommit:
				backend.state = installSnapshot{Present: true, Running: true, Images: target}
			}
			if err := recoverInputInstallation(backend); err != nil {
				t.Fatal(err)
			}
			want := old
			if phase == installCommit {
				want = installSnapshot{Present: true, Running: true, Images: target}
			}
			if backend.state != want || backend.journal != nil {
				t.Fatal("interrupted transaction not reconciled")
			}
		})
	}
}

func TestInstallTransactionReleaseFailureBlocksReplaceRestoreAndStart(t *testing.T) {
	old := installSnapshot{Present: true, Running: true, Images: installTestImages("old")}
	backend := &installTestBackend{state: old, fail: "stop"}
	if err := applyInputInstallation(context.Background(), backend, installTestImages("new")); err != errInstallRecovery {
		t.Fatal("failed release was not quarantined")
	}
	for _, operation := range backend.operations {
		if operation == "replace" || operation == "restore" || operation == "start" || operation == "finish" {
			t.Fatal("unverified release permitted a destructive/restart effect")
		}
	}
	if backend.journal == nil || backend.journal.Phase != installRollbackStop || backend.state != old || backend.locked {
		t.Fatal("recovery evidence or exclusive lock leaked")
	}
}

func TestInstallTransactionCancellationCannotCancelRollback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	old := installSnapshot{Present: true, Running: true, Images: installTestImages("old")}
	backend := &installTestBackend{state: old, cancelAt: "replace", cancel: cancel}
	// Replacement may take effect immediately before cancellation is observed.
	if err := applyInputInstallation(ctx, backend, installTestImages("new")); err != errInstallFailed {
		t.Fatal("cancelled update was not rolled back")
	}
	if !backend.recoveryCtxOK || backend.state != old || backend.journal != nil {
		t.Fatal("cancelled update also cancelled recovery")
	}
}

func TestInstallTransactionReadinessFailureStopsCandidateBeforeRestoring(t *testing.T) {
	old := installSnapshot{Present: true, Running: true, Images: installTestImages("old")}
	for _, fail := range []string{"start", "verify"} {
		backend := &installTestBackend{state: old, fail: fail, failOnce: true}
		if err := applyInputInstallation(context.Background(), backend, installTestImages("new")); err != errInstallFailed {
			t.Fatal("failed candidate readiness was not rolled back")
		}
		if backend.state != old || backend.journal != nil {
			t.Fatal("readiness rollback did not restore old version")
		}
	}
}

func TestInstallTransactionCleanupFailureRetainsCommittedCandidate(t *testing.T) {
	old := installSnapshot{Present: true, Running: true, Images: installTestImages("old")}
	target := installTestImages("new")
	backend := &installTestBackend{state: old, fail: "finish"}
	if err := applyInputInstallation(context.Background(), backend, target); err != errInstallRecovery {
		t.Fatal("cleanup failure hidden")
	}
	want := installSnapshot{Present: true, Running: true, Images: target}
	if backend.state != want || backend.journal == nil || backend.journal.Phase != installCommit {
		t.Fatal("durable commit rolled back after cleanup failure")
	}
	backend.fail = ""
	if err := recoverInputInstallation(backend); err != nil || backend.state != want || backend.journal != nil {
		t.Fatal("committed cleanup could not resume")
	}
}

func TestInstallTransactionRejectsJournalInjectionAndConflictingLock(t *testing.T) {
	old := installSnapshot{Present: true, Running: true, Images: installTestImages("old")}
	for _, backend := range []*installTestBackend{
		{state: old, locked: true},
		{state: old, fail: "load"},
		{state: old, journal: &installJournal{Schema: "other", Phase: installCommit, Previous: old}},
	} {
		if err := applyInputInstallation(context.Background(), backend, installTestImages("new")); err != errInstallRecovery || backend.state != old {
			t.Fatal("untrusted or conflicting recovery changed installation")
		}
		for _, operation := range backend.operations {
			if operation != "lock" && operation != "load" {
				t.Fatal("recovery rejection was followed by an effect")
			}
		}
	}
}

func TestInstallJournalCanonicalAndBounded(t *testing.T) {
	journal := installJournal{Schema: "krasis.input.transaction/v1", Phase: installStage, Target: installTestImages("new")}
	data, err := encodeInstallJournal(journal)
	if err != nil {
		t.Fatal(err)
	}
	if decoded, err := decodeInstallJournal(data); err != nil || decoded != journal {
		t.Fatal("journal roundtrip failed")
	}
	for _, bad := range []string{
		string(data[:len(data)-1]) + `,"phase":"commit"}`,
		string(data[:len(data)-1]) + `,"path":"arbitrary.exe"}`,
		strings.Replace(string(data), `"phase":"stage"`, `"phase":"execute"`, 1),
		`{"schemaVersion":"krasis.input.transaction/v1","phase":"stage"}`,
		string(data) + `{}`,
		strings.Repeat(" ", 2049),
	} {
		if _, err := decodeInstallJournal([]byte(bad)); err != errInstallRecovery {
			t.Fatal("noncanonical journal accepted")
		}
	}
}
