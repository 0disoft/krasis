//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

type batchedEntryInput struct {
	entryPlacementInput
	entries int
}

func (p *batchedEntryInput) EnterCursor(ctx context.Context, x, y int32) (application.CursorEntry, error) {
	p.entries++
	before := p.position
	placement, err := p.PlaceCursor(ctx, x, y)
	return application.CursorEntry{CursorPlacement: placement, BeforeX: before.X, BeforeY: before.Y}, err
}

func TestBatchedEntryAvoidsSnapshotIPCAndRetainsConditionalRollback(t *testing.T) {
	rejectOrdinaryCursorIO(t)
	for _, mode := range []string{"clipped", "mismatch", "not_applied", "unknown"} {
		t.Run(mode, func(t *testing.T) {
			before := point{100, 200}
			port := &batchedEntryInput{entryPlacementInput: entryPlacementInput{delegatedInput: delegatedInput{position: before}, mode: mode}}
			adapter, err := NewInputAdapterWithInjection(10, 1920, port)
			if err != nil {
				t.Fatal(err)
			}
			err = adapter.PrepareEntry(context.Background(), control.ScreenEdgeRight)
			if port.entries != 1 || port.places != 1 || port.reads != 0 || len(port.calls) != 0 {
				t.Fatal("entry added snapshot IPC, native fallback or replay")
			}
			if mode == "clipped" && err != nil {
				t.Fatal(err)
			}
			if mode == "mismatch" {
				assertCursorStage(t, err, "entry", "mismatch")
			}
			if mode == "unknown" || mode == "not_applied" {
				want := application.InjectionOutcomeUnknown
				if mode == "not_applied" {
					want = application.InjectionNotApplied
				}
				if c, known := application.InjectionCertaintyOf(err); !known || c != want {
					t.Fatal("entry certainty changed", err)
				}
			}
			outcome, rollbackErr := adapter.RollbackEntry(context.Background())
			if mode == "unknown" {
				if !errors.Is(rollbackErr, ErrCursorEntryState) || len(port.calls) != 0 {
					t.Fatal("uncertain entry restored an unverified snapshot")
				}
			} else if mode == "not_applied" {
				if rollbackErr != nil || outcome != application.CursorRollbackNotRestored {
					t.Fatal("unapplied entry rolled back", rollbackErr)
				}
			} else if rollbackErr != nil || outcome != application.CursorRollbackRestored || port.position != before {
				t.Fatal("entry snapshot lost conditional rollback", rollbackErr)
			}
		})
	}
}
