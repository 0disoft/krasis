package control

import (
	"errors"
	"testing"
)

func TestReturnIntentRequiresSessionAndEpoch(t *testing.T) {
	if err := (ReturnIntent{SessionID: "session-1", Epoch: "epoch-1"}).Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
	for _, intent := range []ReturnIntent{{}, {SessionID: "session-1"}, {Epoch: "epoch-1"}} {
		if err := intent.Validate(); !errors.Is(err, ErrInvalidReturnIntent) {
			t.Fatalf("Validate(%+v) error = %v", intent, err)
		}
	}
}
