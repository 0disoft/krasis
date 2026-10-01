package inputworker

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"time"
)

var (
	errInstallFailed   = errors.New("input installation failed; previous installation restored")
	errInstallRecovery = errors.New("input installation recovery required")
	errInstallJournal  = errors.New("input installation journal outcome unknown")
)

const installRecoveryTimeout = 20 * time.Second

type installImages struct {
	Agent   [sha256.Size]byte `json:"agent"`
	Service [sha256.Size]byte `json:"service"`
}

func (images installImages) valid() bool {
	return images.Agent != ([sha256.Size]byte{}) && images.Service != ([sha256.Size]byte{})
}

type installSnapshot struct {
	Present bool          `json:"present"`
	Running bool          `json:"running"`
	Images  installImages `json:"images"`
}

func (snapshot installSnapshot) valid() bool {
	if snapshot.Present {
		return snapshot.Images.valid()
	}
	return !snapshot.Running && snapshot.Images == (installImages{})
}

type installPhase string

const (
	installStage           installPhase = "stage"
	installStop            installPhase = "stop"
	installReplace         installPhase = "replace"
	installStart           installPhase = "start"
	installCommit          installPhase = "commit"
	installRollbackStop    installPhase = "rollback_stop"
	installRollbackRestore installPhase = "rollback_restore"
	installRollbackStart   installPhase = "rollback_start"
	installRollbackCommit  installPhase = "rollback_commit"
)

// This record is recovery metadata, never permission to execute its hashes.
// The native backend must authenticate its protected location and the pinned
// images independently. It contains no paths, commands, PIDs or input payloads.
type installJournal struct {
	Schema   string          `json:"schemaVersion"`
	Phase    installPhase    `json:"phase"`
	Previous installSnapshot `json:"previous"`
	Target   installImages   `json:"target"`
}

func (journal installJournal) valid() bool {
	if journal.Schema != "krasis.input.transaction/v1" || !journal.Previous.valid() {
		return false
	}
	if !journal.Target.valid() && (journal.Target != (installImages{}) || !journal.Previous.Present) {
		return false
	}
	switch journal.Phase {
	case installStage, installStop, installReplace, installStart, installCommit,
		installRollbackStop, installRollbackRestore, installRollbackStart, installRollbackCommit:
		return true
	}
	return false
}

func encodeInstallJournal(journal installJournal) ([]byte, error) {
	if !journal.valid() {
		return nil, errInstallRecovery
	}
	return json.Marshal(journal)
}

func decodeInstallJournal(data []byte) (installJournal, error) {
	var journal installJournal
	if len(data) > 2048 || json.Unmarshal(data, &journal) != nil {
		return installJournal{}, errInstallRecovery
	}
	canonical, err := encodeInstallJournal(journal)
	// Our own writer uses a single canonical representation. Reject duplicate,
	// unknown or missing fields instead of letting JSON last-key-wins pick trust.
	if err != nil || !bytes.Equal(data, canonical) {
		return installJournal{}, errInstallRecovery
	}
	return journal, nil
}

// All methods operate on fixed protected locations. Implementations must be
// idempotent after a process crash, bounded by ctx, and hold pinned handles.
// Stage prepares the candidate AND a verified rollback copy before Stop.
// Stop succeeds only after a clean worker release receipt; a stopped process
// alone is insufficient. Replace/Restore include the fixed SCM configuration.
// Start confirms readiness; Verify checks both image trust and service state.
// Finish removes only transaction-owned artifacts, deleting the journal last.
// The embedded-payload installer owns this interface; no portable-agent endpoint
// exposes it.
type installBackend interface {
	Lock(context.Context) (func(), error)
	Load(context.Context) (*installJournal, error)
	Snapshot(context.Context) (installSnapshot, error)
	Save(context.Context, installJournal) error
	Stage(context.Context, installJournal) error
	Stop(context.Context) error
	Replace(context.Context, installImages) error
	Start(context.Context) error
	Verify(context.Context, installSnapshot) error
	Restore(context.Context, installSnapshot) error
	Finish(context.Context) error
}

// applyInputInstallation is private and not wired to product updates yet.
// The native installer must derive target from its independently
// authorized payload, never from received BUILD.json/checksums or RPC arguments.
// A zero target removes an existing installation through the same recovery path.
func applyInputInstallation(ctx context.Context, backend installBackend, target installImages) error {
	if ctx == nil || backend == nil || (!target.valid() && target != (installImages{})) {
		return errInstallRecovery
	}
	if ctx.Err() != nil {
		return errInstallRecovery
	}
	unlock, err := backend.Lock(ctx)
	if err != nil || unlock == nil {
		return errInstallRecovery
	}
	defer unlock()
	if err := recoverInputInstallation(backend); err != nil {
		return err
	}
	previous, err := backend.Snapshot(ctx)
	if err != nil || !previous.valid() {
		return errInstallRecovery
	}
	if target == (installImages{}) && !previous.Present {
		return nil
	}
	journal := installJournal{Schema: "krasis.input.transaction/v1", Phase: installStage, Previous: previous, Target: target}
	if err := backend.Save(ctx, journal); err != nil {
		return errInstallJournal
	}
	steps := []struct {
		phase installPhase
		run   func() error
	}{
		{installStage, func() error { return backend.Stage(ctx, journal) }},
		{installStop, func() error { return backend.Stop(ctx) }},
		{installReplace, func() error { return backend.Replace(ctx, target) }},
		{installStart, func() error {
			if target.valid() {
				if err := backend.Start(ctx); err != nil {
					return err
				}
			}
			return backend.Verify(ctx, installSnapshot{Present: target.valid(), Running: target.valid(), Images: target})
		}},
	}
	for index, step := range steps {
		if ctx.Err() != nil {
			if err := recoverInputInstallation(backend); err != nil {
				return err
			}
			return errInstallFailed
		}
		if index > 0 {
			journal.Phase = step.phase
			if err := backend.Save(ctx, journal); err != nil {
				// An atomic journal replacement may have succeeded before an I/O
				// error. Reconcile it on the next run; do not speculate or replay.
				return errInstallJournal
			}
		}
		err := ctx.Err()
		if err == nil {
			err = step.run()
		}
		if err != nil {
			if err := recoverInputInstallation(backend); err != nil {
				return err
			}
			return errInstallFailed
		}
	}
	if ctx.Err() != nil {
		if err := recoverInputInstallation(backend); err != nil {
			return err
		}
		return errInstallFailed
	}
	journal.Phase = installCommit
	if err := backend.Save(ctx, journal); err != nil {
		return errInstallJournal
	}
	// Cancellation after the durable commit must not turn an installed version
	// into a speculative rollback. Completion revalidates the committed state.
	return recoverInputInstallation(backend)
}

// Caller holds the exclusive install lock. Recovery gets an independent bounded
// context so cancelling an update cannot also cancel its key-up/rollback path.
func recoverInputInstallation(backend installBackend) error {
	ctx, cancel := context.WithTimeout(context.Background(), installRecoveryTimeout)
	defer cancel()
	journal, err := backend.Load(ctx)
	if err != nil {
		return errInstallRecovery
	}
	if journal == nil {
		return nil
	}
	if !journal.valid() {
		return errInstallRecovery
	}
	if journal.Phase == installCommit {
		state := installSnapshot{Present: journal.Target.valid(), Running: journal.Target.valid(), Images: journal.Target}
		if backend.Verify(ctx, state) != nil || backend.Finish(ctx) != nil {
			return errInstallRecovery
		}
		return nil
	}
	if journal.Phase == installStage {
		// Stage cannot mutate the active installation, even when interrupted.
		if backend.Verify(ctx, journal.Previous) != nil || backend.Finish(ctx) != nil {
			return errInstallRecovery
		}
		return nil
	}
	// Persist the rollback direction before doing anything, so a crash never
	// causes the next run to resume activation of an uncommitted candidate.
	if journal.Phase == installStop || journal.Phase == installReplace || journal.Phase == installStart {
		journal.Phase = installRollbackStop
		if backend.Save(ctx, *journal) != nil {
			return errInstallJournal
		}
	}
	for journal.Phase != installRollbackCommit {
		var next installPhase
		switch journal.Phase {
		case installRollbackStop:
			if backend.Stop(ctx) != nil {
				// Never replace files or restart another backend after an
				// unverified input release, even to restore the old version.
				return errInstallRecovery
			}
			next = installRollbackRestore
		case installRollbackRestore:
			if backend.Restore(ctx, journal.Previous) != nil {
				return errInstallRecovery
			}
			next = installRollbackStart
		case installRollbackStart:
			if journal.Previous.Running && backend.Start(ctx) != nil {
				return errInstallRecovery
			}
			if backend.Verify(ctx, journal.Previous) != nil {
				return errInstallRecovery
			}
			next = installRollbackCommit
		default:
			return errInstallRecovery
		}
		journal.Phase = next
		if backend.Save(ctx, *journal) != nil {
			return errInstallJournal
		}
	}
	if backend.Verify(ctx, journal.Previous) != nil || backend.Finish(ctx) != nil {
		return errInstallRecovery
	}
	return nil
}
