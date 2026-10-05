//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

type entryPlacementInput struct {
	delegatedInput
	places, reads int
	mode          string
}

func (p *entryPlacementInput) PlaceCursor(_ context.Context, x, y int32) (application.CursorPlacement, error) {
	p.places++
	if p.mode == "not_applied" {
		return application.CursorPlacement{}, &application.InjectionError{Certainty: application.InjectionNotApplied}
	}
	p.position = point{X: x + 5, Y: y}
	if p.mode == "unknown" {
		return application.CursorPlacement{}, &application.InjectionError{Certainty: application.InjectionOutcomeUnknown}
	}
	placement := application.CursorPlacement{TargetX: x + 5, TargetY: y, X: x + 5, Y: y}
	if p.mode == "mismatch" {
		placement.X++
		p.position.X++
	}
	return placement, nil
}

func (p *entryPlacementInput) CursorPosition(context.Context) (int32, int32, error) {
	p.reads++
	return p.position.X, p.position.Y, nil
}

func TestProtectedEntryUsesVerifiedEffectiveTargetAndRetainsRollbackOwnership(t *testing.T) {
	rejectOrdinaryCursorIO(t)
	for _, mode := range []string{"clipped", "mismatch", "unknown", "not_applied"} {
		t.Run(mode, func(t *testing.T) {
			before := point{X: 100, Y: 200}
			port := &entryPlacementInput{delegatedInput: delegatedInput{position: before}, mode: mode}
			adapter, err := NewInputAdapterWithInjection(10, 1920, port)
			if err != nil {
				t.Fatal(err)
			}
			err = adapter.PrepareEntry(context.Background(), control.ScreenEdgeRight)
			if port.places != 1 || port.reads != 1 || len(port.calls) != 0 {
				t.Fatal("entry split, replayed or used fallback query")
			}
			if mode == "clipped" {
				if err != nil || !adapter.entry.enteredKnown || adapter.entry.enteredX != port.position.X {
					t.Fatal("verified effective target refused", err)
				}
			} else if mode == "mismatch" {
				assertCursorStage(t, err, "entry", "mismatch")
				if !adapter.entry.enteredKnown {
					t.Fatal("observed mismatch lost rollback ownership")
				}
			} else {
				want := application.InjectionOutcomeUnknown
				if mode == "not_applied" {
					want = application.InjectionNotApplied
				}
				if c, known := application.InjectionCertaintyOf(err); !known || c != want {
					t.Fatal("entry receipt lost certainty", err)
				}
			}
			outcome, rollbackErr := adapter.RollbackEntry(context.Background())
			if mode == "unknown" {
				if !errors.Is(rollbackErr, ErrCursorEntryState) || len(port.calls) != 0 {
					t.Fatal("unknown placement blindly restored")
				}
			} else if mode == "not_applied" {
				if rollbackErr != nil || outcome != application.CursorRollbackNotRestored || port.position != before {
					t.Fatal("unapplied placement changed cursor")
				}
			} else if rollbackErr != nil || outcome != application.CursorRollbackRestored || port.position != before || port.places != 1 {
				t.Fatal("owned rollback lost or entry replayed", rollbackErr)
			}
		})
	}
}
