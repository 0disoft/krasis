package crashguard

import (
	"bytes"
	"errors"
	"testing"
)

func TestDisarmRejectedAckPreservesRecoveryFailure(t *testing.T) {
	var ack bytes.Buffer
	if err := writeAck(&ack, operationDisarm, statusRejected); err != nil {
		t.Fatal(err)
	}
	client, err := NewClient(&bytes.Buffer{}, &ack)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Disarm(); !errors.Is(err, ErrRecoveryFailed) {
		t.Fatalf("lost recovery failure: %v", err)
	}
}
