//go:build windows

package inputworker

import (
	"context"
	"testing"
)

type stoppedImageBackend struct {
	installTestBackend
	receipt         serviceRecord
	active, retired installImages
}

func (b *stoppedImageBackend) Stop(ctx context.Context) error {
	_, err := stoppedInstallDigest(ctx, b.journal, b.receipt, func(kind installDirectory) (installImages, bool, error) {
		switch kind {
		case installActive:
			return b.active, b.active.valid(), nil
		case installRetired:
			return b.retired, b.retired.valid(), nil
		case installBackup:
			return b.backup.Images, b.backup.Present, nil
		}
		return installImages{}, false, nil
	})
	if err != nil {
		return err
	}
	return b.installTestBackend.Stop(ctx)
}

func TestInterruptedReplacementRestoresTheLastCleanService(t *testing.T) {
	for _, phase := range []serviceRecordPhase{serviceClean, serviceStarting, serviceReady, serviceFailed} {
		t.Run(string(rune('0'+phase)), func(t *testing.T) {
			previous := installSnapshot{Present: true, Running: true, Images: installTestImages("previous")}
			target := installTestImages("target")
			b := &stoppedImageBackend{installTestBackend: installTestBackend{
				journal: &installJournal{Schema: "krasis.input.transaction/v1", Phase: installReplace, Previous: previous, Target: target},
				state:   installSnapshot{Present: true, Images: target}, backup: previous, staged: true},
				active: target, retired: previous.Images,
				receipt: serviceRecord{Phase: phase, PID: 42, Run: Binding{1}, Digest: previous.Images.Service}}
			err := recoverInputInstallation(b)
			if phase == serviceClean {
				if err != nil || b.state != previous || b.journal != nil || b.staged {
					t.Fatal("clean previous generation not restored", err, b.state, b.journal)
				}
			} else if err == nil || b.state.Images != target || b.journal == nil || b.journal.Phase != installRollbackStop {
				t.Fatal("unconfirmed input release allowed rollback", err, b.state, b.journal)
			}
		})
	}
}

func TestStoppedGenerationRequiresJournalAndVerifiedImages(t *testing.T) {
	previous, target := installTestImages("previous"), installTestImages("target")
	journal := &installJournal{Schema: "krasis.input.transaction/v1", Phase: installRollbackStop, Previous: installSnapshot{Present: true, Running: true, Images: previous}, Target: target}
	record := serviceRecord{Phase: serviceClean, PID: 42, Run: Binding{1}, Digest: previous.Service}
	for _, mode := range []string{"no journal", "invalid journal", "unrelated image", "unknown receipt", "tampered image"} {
		t.Run(mode, func(t *testing.T) {
			j := *journal
			r := record
			candidate := &j
			if mode == "no journal" {
				candidate = nil
			}
			if mode == "invalid journal" {
				j.Schema = "other"
			}
			if mode == "unknown receipt" {
				r.Digest = installTestImages("unknown").Service
			}
			_, err := stoppedInstallDigest(context.Background(), candidate, r, func(kind installDirectory) (installImages, bool, error) {
				if kind == installActive {
					if mode == "tampered image" {
						return installImages{}, true, ErrAuthority
					}
					return target, true, nil
				}
				if mode == "unrelated image" {
					return installImages{Agent: target.Agent, Service: previous.Service}, true, nil
				}
				return previous, true, nil
			})
			if err == nil {
				t.Fatal("unverified generation selected")
			}
		})
	}
}
