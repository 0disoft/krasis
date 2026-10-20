//go:build windows

package inputworker

import (
	"context"
	"errors"
	"testing"
)

func TestStoppedRepairStartsOldServiceWithDurableMatchingJournal(t *testing.T) {
	images := installTestImages("old installed image")
	previous := installSnapshot{Present: true, Images: images}
	backend := &installTestBackend{state: previous}
	record := serviceRecord{Phase: serviceFailed, PID: 42, Run: Binding{1}, Digest: images.Service}
	starts, stops := 0, 0
	err := repairStoppedServiceTransaction(context.Background(), record, images, backend,
		func(uint32) (bool, error) { return true, nil },
		func(context.Context) error {
			starts++
			// An old service rejects the held installer lock without a journal.
			// Its unchanged startup policy must accept only the verified old image.
			args := []string{ServiceName, installServiceStartArgument}
			if backend.journal == nil || !serviceStartAllowed(args, images, backend.journal) ||
				serviceStartAllowed(args, installTestImages("unrelated"), backend.journal) ||
				serviceStartAllowed([]string{ServiceName}, images, backend.journal) {
				t.Fatal("repair start has no matching durable authorization")
			}
			backend.state.Running = true
			return nil
		}, func(context.Context) error {
			stops++
			backend.state.Running = false
			return nil
		})
	if err != nil || starts != 1 || stops != 1 || backend.journal != nil || backend.state != previous {
		t.Fatal("repair did not finish one verified old-image cycle", err, starts, stops, backend.journal)
	}
}

func TestStoppedRepairRetainsJournalAfterUncertainStartOrStop(t *testing.T) {
	for _, failStart := range []bool{true, false} {
		images := installTestImages("old")
		backend := &installTestBackend{state: installSnapshot{Present: true, Images: images}}
		record := serviceRecord{Phase: serviceFailed, PID: 42, Run: Binding{1}, Digest: images.Service}
		fault := errors.New("uncertain service outcome")
		err := repairStoppedServiceTransaction(context.Background(), record, images, backend,
			func(uint32) (bool, error) { return true, nil },
			func(context.Context) error {
				backend.state.Running = true // An error need not prove SCM never started.
				if failStart {
					return fault
				}
				return nil
			}, func(context.Context) error { return fault })
		if !errors.Is(err, fault) || backend.journal == nil {
			t.Fatal("uncertain repair erased its durable evidence", err)
		}
		// Recovery must not replace files or restart an unconfirmed running owner.
		backend.fail = "stop"
		if err := recoverInputInstallation(backend); err == nil || backend.journal == nil {
			t.Fatal("uncertain repair was silently committed", failStart, err)
		}
	}
}
