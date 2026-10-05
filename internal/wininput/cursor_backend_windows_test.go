//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

func rejectOrdinaryCursorIO(t *testing.T) {
	oldGet, oldSet := callGetCursorPos, callSetCursorPos
	callGetCursorPos = func(*point) (uintptr, uintptr, error) {
		t.Fatal("protected path read ordinary cursor")
		return 0, 0, nil
	}
	callSetCursorPos = func(...uintptr) (uintptr, uintptr, error) {
		t.Fatal("protected path wrote ordinary cursor")
		return 0, 0, nil
	}
	t.Cleanup(func() { callGetCursorPos, callSetCursorPos = oldGet, oldSet })
}

func TestProtectedCursorEntryAndRollbackNeverUseOrdinaryCursorIO(t *testing.T) {
	rejectOrdinaryCursorIO(t)
	port := &delegatedInput{position: point{X: 100, Y: 200}}
	adapter, err := NewInputAdapterWithInjection(10, systemMetric(smCXVIRTUALSCREEN), port)
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	for i := 0; i < 20; i++ {
		before := port.position
		if err := adapter.PrepareEntry(ctx, control.ScreenEdgeRight); err != nil {
			t.Fatal(err)
		}
		if i%2 == 0 {
			outcome, err := adapter.RollbackEntry(ctx)
			if err != nil || outcome != application.CursorRollbackRestored || port.position != before {
				t.Fatal("protected rollback lost owned position", err)
			}
		} else {
			adapter.CommitEntry()
		}
		if adapter.entry.pending {
			t.Fatal("entry transaction remained pending")
		}
	}
}

type faultCursorInput struct {
	delegatedInput
	setErr error
}

func (p *faultCursorInput) SetCursorPosition(context.Context, int32, int32) error { return p.setErr }

func TestProtectedEntryLostCursorACKRequiresRecoveryAndNeverFallsBack(t *testing.T) {
	rejectOrdinaryCursorIO(t)
	for _, certainty := range []application.InjectionCertainty{application.InjectionOutcomeUnknown, application.InjectionNotApplied} {
		cause := &application.InjectionError{Certainty: certainty}
		port := &faultCursorInput{setErr: cause}
		adapter, err := NewInputAdapterWithInjection(10, systemMetric(smCXVIRTUALSCREEN), port)
		if err != nil {
			t.Fatal(err)
		}
		err = adapter.PrepareEntry(context.Background(), control.ScreenEdgeRight)
		if !errors.Is(err, cause) {
			t.Fatal("cursor write failure was hidden")
		}
		outcome, rollbackErr := adapter.RollbackEntry(context.Background())
		if certainty == application.InjectionOutcomeUnknown {
			if !errors.Is(rollbackErr, ErrCursorEntryState) {
				t.Fatal("unknown cursor write was reported as unnecessary rollback")
			}
		} else if rollbackErr != nil || outcome != application.CursorRollbackNotRestored {
			t.Fatal("known unapplied cursor write retained uncertain state")
		}
	}
}

type positionedInput struct {
	delegatedInput
	moves, reads int
}

func (p *positionedInput) MoveWithPosition(context.Context, int32, int32) (int32, int32, error) {
	p.moves++
	return p.position.X, p.position.Y, p.err
}
func (p *positionedInput) CursorPosition(context.Context) (int32, int32, error) {
	p.reads++
	return p.position.X, p.position.Y, nil
}

func TestProtectedMotionUsesACKPositionWithoutASecondQuery(t *testing.T) {
	rejectOrdinaryCursorIO(t)
	port := &positionedInput{}
	adapter, err := NewInputAdapterWithInjection(10, 1920, port)
	if err != nil {
		t.Fatal(err)
	}
	if err := adapter.Move(context.Background(), 1, 1); err != nil {
		t.Fatal(err)
	}
	if port.moves != 1 || port.reads != 0 {
		t.Fatal("motion repeated cursor query")
	}
}

func TestNativeWorkerCursorSetRejectsOutOfBoundsAndCancelledRequest(t *testing.T) {
	old := callSetCursorPos
	calls := 0
	callSetCursorPos = func(...uintptr) (uintptr, uintptr, error) { calls++; return 1, 0, nil }
	t.Cleanup(func() { callSetCursorPos = old })
	port := nativeInjectionPort{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	x, y := systemMetric(smXVIRTUALSCREEN), systemMetric(smYVIRTUALSCREEN)
	if port.SetCursorPosition(ctx, x, y) == nil || calls != 0 {
		t.Fatal("cancelled cursor write was applied")
	}
	if port.SetCursorPosition(context.Background(), x-1, y) == nil || calls != 0 {
		t.Fatal("out-of-bounds cursor write was applied")
	}
	if err := port.SetCursorPosition(context.Background(), x, y); err != nil || calls != 1 {
		t.Fatal("valid worker cursor target refused", err)
	}
}
