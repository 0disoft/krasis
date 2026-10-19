package inputworker

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

type switchingCursorInput struct {
	recordingInput
	moves, reads int
	readFailure  error
	persistent   bool
	cancel       context.CancelFunc
}

func (p *switchingCursorInput) Move(context.Context, int32, int32) error { p.moves++; return nil }
func (p *switchingCursorInput) SetCursorPosition(context.Context, int32, int32) error {
	panic("observation recovery must never place a cursor")
}
func (p *switchingCursorInput) CursorPosition(context.Context) (int32, int32, error) {
	p.reads++
	if p.cancel != nil {
		p.cancel()
	}
	if p.reads == 1 || p.persistent {
		return 0, 0, p.readFailure
	}
	return 12, 34, nil
}

func TestMoveReadbackRechecksScopeOnceWithoutReplayingInput(t *testing.T) {
	for _, mode := range []string{"recovered", "persistent", "authority", "cleanup", "cancelled", "no_scope", "unclassified"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			input := &switchingCursorInput{readFailure: application.ErrCursorObservationUnavailable, persistent: mode == "persistent"}
			if mode == "cancelled" {
				input.cancel = cancel
			}
			if mode == "unclassified" {
				input.readFailure = errors.New("unclassified observation failure")
			}
			checks := 0
			if mode != "no_scope" {
				ctx = context.WithValue(ctx, cursorObservationScopeKey{}, executor(func(actual context.Context, read func() error) error {
					checks++
					if actual != ctx || input.moves != 1 || input.reads != 1 {
						t.Fatal("scope check did not precede only the second observation")
					}
					if mode == "authority" {
						return ErrAuthority
					}
					if mode == "cleanup" {
						return inputdesktop.ErrCleanup
					}
					return read()
				}))
			}
			var ack response
			var pressed control.PressedState
			err := applyCommand(ctx, input, &pressed, request{op: opMoveRead, x: 1}, &ack)
			if input.moves != 1 {
				t.Fatal("movement was replayed")
			}
			if mode == "recovered" {
				if err != nil || input.reads != 2 || checks != 1 || ack.x != 12 || ack.y != 34 || ack.failureStage != 0 {
					t.Fatal("actual readback did not recover", err, ack)
				}
				return
			}
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown || ack.failureStage != failureStageCursorReadback {
				t.Fatal("unconfirmed move became success", err, ack)
			}
			wantReads, wantChecks := 1, 0
			if mode == "persistent" {
				wantReads, wantChecks = 2, 1
			}
			if mode == "authority" || mode == "cleanup" {
				wantChecks = 1
			}
			if input.reads != wantReads || checks != wantChecks || !errors.Is(err, input.readFailure) {
				t.Fatal("unbounded retry or lost original failure", input.reads, checks, err)
			}
			if mode == "authority" && !errors.Is(err, ErrAuthority) || mode == "cleanup" && !errors.Is(err, inputdesktop.ErrCleanup) {
				t.Fatal("scope failure was hidden", err)
			}
		})
	}
}
