//go:build windows

package wininput

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

type stagedCursorInput struct {
	delegatedInput
	reads, writes, readFail, writeFail, mismatch int
	cause                                        error
}

func (p *stagedCursorInput) CursorPosition(context.Context) (int32, int32, error) {
	p.reads++
	if p.reads == p.readFail {
		return 0, 0, p.cause
	}
	x := p.position.X
	if p.reads == p.mismatch {
		x++
	}
	return x, p.position.Y, nil
}

func (p *stagedCursorInput) SetCursorPosition(_ context.Context, x, y int32) error {
	p.writes++
	if p.writes == p.writeFail {
		return p.cause
	}
	p.position = point{X: x, Y: y}
	return nil
}

func assertCursorStage(t *testing.T, err error, operation, stage string) {
	t.Helper()
	var failure *application.CursorTransactionFailure
	if !errors.As(err, &failure) || failure.Operation != operation || failure.Stage != stage || failure.Cause == nil {
		t.Fatalf("missing %s/%s stage: %v", operation, stage, err)
	}
}

func TestEntryFailureStageIdentifiesFirstFaultWithoutReplayingInput(t *testing.T) {
	rejectOrdinaryCursorIO(t)
	for _, stage := range []string{"context", "state", "snapshot", "geometry", "set", "verify", "mismatch"} {
		t.Run(stage, func(t *testing.T) {
			cause := &application.InjectionError{Certainty: application.InjectionOutcomeUnknown}
			port := &stagedCursorInput{cause: cause}
			adapter, err := NewInputAdapterWithInjection(10, 1920, port)
			if err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			edge := control.ScreenEdgeRight
			switch stage {
			case "context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "state":
				adapter.entry.pending = true
			case "snapshot":
				port.readFail = 1
			case "geometry":
				edge = "INVALID"
			case "set":
				port.writeFail = 1
			case "verify":
				port.readFail = 2
			case "mismatch":
				port.mismatch = 2
			}
			err = adapter.PrepareEntry(ctx, edge)
			assertCursorStage(t, err, "entry", stage)
			if port.writes > 1 {
				t.Fatal("entry input replayed")
			}
			if stage == "snapshot" || stage == "set" || stage == "verify" {
				if !errors.Is(err, cause) {
					t.Fatal("original outcome lost")
				}
			}
			if stage == "set" || stage == "verify" || stage == "mismatch" {
				if !adapter.entry.pending {
					t.Fatal("pending recovery lost")
				}
			}
		})
	}
}

func TestRollbackFailureKeepsOwnStageAndClearsTransaction(t *testing.T) {
	rejectOrdinaryCursorIO(t)
	for _, stage := range []string{"context", "state", "unverified", "inspect", "restore", "verify", "mismatch"} {
		t.Run(stage, func(t *testing.T) {
			cause := &application.InjectionError{Certainty: application.InjectionNotApplied}
			port := &stagedCursorInput{cause: cause}
			adapter, err := NewInputAdapterWithInjection(10, 1920, port)
			if err != nil {
				t.Fatal(err)
			}
			if err := adapter.PrepareEntry(context.Background(), control.ScreenEdgeRight); err != nil {
				t.Fatal(err)
			}
			ctx := context.Background()
			switch stage {
			case "context":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "state":
				adapter.entry = nil
			case "unverified":
				adapter.entry.enteredKnown = false
			case "inspect":
				port.readFail = 3
			case "restore":
				port.writeFail = 2
			case "verify":
				port.readFail = 4
			case "mismatch":
				port.mismatch = 4
			}
			_, err = adapter.RollbackEntry(ctx)
			assertCursorStage(t, err, "rollback", stage)
			if adapter.entry != nil && adapter.entry.pending {
				t.Fatal("rollback state retained")
			}
			if port.writes > 2 {
				t.Fatal("rollback input replayed")
			}
			if stage == "inspect" || stage == "restore" || stage == "verify" {
				if !errors.Is(err, cause) {
					t.Fatal("rollback cause lost")
				}
			}
		})
	}
}
