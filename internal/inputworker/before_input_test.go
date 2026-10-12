package inputworker

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
)

func TestBeforeInputFailureIsNotAppliedAndNeverCallsInput(t *testing.T) {
	for _, cause := range []error{ErrAuthority, errors.New("desktop preparation failed"), context.Canceled} {
		execute := beforeInputCertainty(func(context.Context, func() error) error { return cause })
		err := execute(context.Background(), func() error { t.Fatal("input dispatched after preparation failure"); return nil })
		certainty, requested, inserted := describeFailure(err)
		if !errors.Is(err, cause) || certainty != 1 || requested != 0 || inserted != 0 {
			t.Fatal("pre-dispatch failure lost certainty", err, certainty, requested, inserted)
		}
	}
}

func TestStartedInputFailureKeepsItsCertaintyAndIsNeverReplayed(t *testing.T) {
	for _, cause := range []error{
		&application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Requested: 2, Inserted: 0},
		&application.InjectionError{Certainty: application.InjectionPartiallyApplied, Requested: 2, Inserted: 1},
		errors.New("post-dispatch failure"),
	} {
		calls := 0
		execute := beforeInputCertainty(func(_ context.Context, operation func() error) error {
			_ = operation()
			return cause
		})
		err := execute(context.Background(), func() error { calls++; return cause })
		if err != cause || calls != 1 {
			t.Fatal("dispatch was replayed or reclassified", err, calls)
		}
	}
}
