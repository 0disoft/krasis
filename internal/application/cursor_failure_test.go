package application

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestCursorFailurePreservesRecoveryOutcomeAndDoesNotEchoCause(t *testing.T) {
	cause := &InjectionError{Certainty: InjectionOutcomeUnknown, Cause: errors.New("SECRET coordinates")}
	failure := &CursorTransactionFailure{Operation: "entry", Stage: "set", Cause: cause}
	wrapped := fmt.Errorf("entry failed: %w", failure)
	if !errors.Is(wrapped, cause) || strings.Contains(failure.Error(), "SECRET") {
		t.Fatal("cause lost or echoed")
	}
	certainty, known := InjectionCertaintyOf(wrapped)
	if !known || certainty != InjectionOutcomeUnknown {
		t.Fatal("diagnostic changed input outcome")
	}
	var nilFailure *CursorTransactionFailure
	if nilFailure.Unwrap() != nil {
		t.Fatal("nil failure has a cause")
	}
}

func TestCursorStageRejectsCrossOperationAndArbitraryValues(t *testing.T) {
	for _, value := range [][2]string{{"entry", "restore"}, {"rollback", "snapshot"}, {"move", "set"}, {"entry", "SECRET"}, {"entry", ""}} {
		if ValidCursorFailureStage(value[0], value[1]) {
			t.Fatal("invalid stage accepted", value)
		}
	}
}
