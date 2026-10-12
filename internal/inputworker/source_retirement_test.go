package inputworker

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
)

func TestSourceRetirementAndCancellationRacePreservesConfirmedJoin(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err, confirmed := sourceRetirementResult(ctx, true, nil); !confirmed || err != context.Canceled {
		t.Fatal("confirmed idle refresh became a protected-session restart", err)
	}
	if err, confirmed := sourceRetirementResult(context.Background(), true, nil); !confirmed || err != wininput.ErrProtectedInput {
		t.Fatal("spontaneous retirement lost its transition", err)
	}
	for _, failure := range []error{ErrRecovery, wininput.ErrEmergencyRelease} {
		if _, confirmed := sourceRetirementResult(ctx, true, failure); confirmed {
			t.Fatal("failed retirement accepted", failure)
		}
	}
	if _, confirmed := sourceRetirementResult(ctx, false, nil); confirmed {
		t.Fatal("missing retirement accepted")
	}
}

func TestSourceTransitionRequiresEveryCleanupCauseToBeKnown(t *testing.T) {
	if !sourceTransitionOnly(errors.Join(context.Canceled, inputdesktop.ErrInactive)) {
		t.Fatal("clean desktop retirement classified as uncertain")
	}
	for _, failure := range []error{ErrRecovery, inputdesktop.ErrCleanup, errors.New("native close failed"), context.DeadlineExceeded} {
		if sourceTransitionOnly(errors.Join(context.Canceled, inputdesktop.ErrInactive, failure)) {
			t.Fatal("cleanup uncertainty hidden", failure)
		}
	}
	if !sourceTransitionOnly(errors.Join(context.Canceled, context.DeadlineExceeded), true) || sourceTransitionOnly(errors.Join(context.DeadlineExceeded, inputdesktop.ErrCleanup), true) {
		t.Fatal("owned expiry hid native cleanup failure")
	}
}
