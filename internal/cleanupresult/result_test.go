package cleanupresult

import (
	"errors"
	"fmt"
	"testing"
)

type sliceError []int

func (sliceError) Error() string { return "non-comparable error" }

func TestConfirmationNeverHidesAdditionalCleanupFailure(t *testing.T) {
	cause, failure := errors.New("stop"), errors.New("cleanup")
	for _, e := range []error{nil, cause, errors.Join(cause, cause)} {
		if !OnlyCause(e, cause) {
			t.Fatal("cause contract rejected", e)
		}
	}
	for _, e := range []error{failure, errors.Join(cause, failure), fmt.Errorf("cleanup: %w", cause), sliceError{1}} {
		if OnlyCause(e, cause) {
			t.Fatal("cleanup accepted as cause", e)
		}
	}
	if OnlyCause(sliceError{1}, sliceError{1}) {
		t.Fatal("non-comparable unknown proof accepted")
	}
	stop := Confirm(cause)
	if !errors.Is(stop, cause) || stop.Error() != cause.Error() || !Confirmed(stop) || !Confirmed(errors.Join(stop, nil)) {
		t.Fatal("lost original cause or confirmation")
	}
	for _, e := range []error{nil, cause, errors.Join(stop, failure), fmt.Errorf("unknown cleanup: %w", stop)} {
		if Confirmed(e) {
			t.Fatal("extra failure accepted", e)
		}
	}
}
