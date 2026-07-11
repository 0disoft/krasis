package control

import (
	"errors"
	"math"
	"testing"
)

func TestOutboundStreamAllocatesMonotonicEnvelopes(t *testing.T) {
	stream, err := NewOutboundStream("session-1", "epoch-1")
	if err != nil {
		t.Fatalf("NewOutboundStream() error = %v", err)
	}
	for want := uint64(1); want <= 2; want++ {
		envelope, next, err := stream.Next()
		if err != nil {
			t.Fatalf("Next() error = %v", err)
		}
		if envelope.SessionID != "session-1" || envelope.Epoch != "epoch-1" || envelope.Sequence != want {
			t.Fatalf("envelope = %+v", envelope)
		}
		stream = next
	}
}

func TestOutboundStreamRejectsInvalidOrExhaustedState(t *testing.T) {
	if _, err := NewOutboundStream("", "epoch-1"); !errors.Is(err, ErrInvalidOutboundStream) {
		t.Fatalf("NewOutboundStream() error = %v", err)
	}
	_, _, err := (OutboundStream{SessionID: "session-1", Epoch: "epoch-1", NextSequence: math.MaxUint64}).Next()
	if !errors.Is(err, ErrStreamExhausted) {
		t.Fatalf("Next() error = %v, want ErrStreamExhausted", err)
	}
}
