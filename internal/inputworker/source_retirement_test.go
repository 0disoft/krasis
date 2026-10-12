package inputworker

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

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
