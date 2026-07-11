package control

import (
	"errors"
	"math"
	"testing"
)

func TestInputStreamAcceptsOnlyExactNextSequence(t *testing.T) {
	stream, err := NewInputStream("session-1", "epoch-1")
	if err != nil {
		t.Fatalf("NewInputStream returned error: %v", err)
	}
	next, err := stream.Accept(InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1})
	if err != nil {
		t.Fatalf("Accept returned error: %v", err)
	}
	if next.NextSequence != 2 || stream.NextSequence != 1 {
		t.Fatalf("unexpected stream snapshots: before=%+v after=%+v", stream, next)
	}
}

func TestInputStreamRejectsUnsafeEnvelopesWithoutAdvancing(t *testing.T) {
	stream, err := NewInputStream("session-2", "epoch-new")
	if err != nil {
		t.Fatalf("NewInputStream returned error: %v", err)
	}
	tests := []struct {
		name     string
		envelope InputEnvelope
		want     error
	}{
		{name: "stale session", envelope: InputEnvelope{SessionID: "session-old", Epoch: "epoch-new", Sequence: 1}, want: ErrStaleSession},
		{name: "stale epoch", envelope: InputEnvelope{SessionID: "session-2", Epoch: "epoch-old", Sequence: 1}, want: ErrStaleEpoch},
		{name: "duplicate", envelope: InputEnvelope{SessionID: "session-2", Epoch: "epoch-new", Sequence: 0}, want: ErrDuplicateInput},
		{name: "gap", envelope: InputEnvelope{SessionID: "session-2", Epoch: "epoch-new", Sequence: 2}, want: ErrInputSequenceGap},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			next, acceptErr := stream.Accept(test.envelope)
			if !errors.Is(acceptErr, test.want) {
				t.Fatalf("error = %v, want %v", acceptErr, test.want)
			}
			if next != (InputStream{}) {
				t.Fatalf("rejected envelope produced a next stream: %+v", next)
			}
		})
	}
}

func TestInputStreamRequiresNewEpochAfterReconnect(t *testing.T) {
	oldStream, err := NewInputStream("session-old", "epoch-old")
	if err != nil {
		t.Fatalf("NewInputStream returned error: %v", err)
	}
	newStream, err := NewInputStream("session-new", "epoch-new")
	if err != nil {
		t.Fatalf("NewInputStream returned error: %v", err)
	}
	_, err = newStream.Accept(InputEnvelope{SessionID: oldStream.SessionID, Epoch: oldStream.Epoch, Sequence: 1})
	if !errors.Is(err, ErrStaleSession) {
		t.Fatalf("error = %v, want ErrStaleSession", err)
	}
}

func TestInputStreamRefusesSequenceWraparound(t *testing.T) {
	stream := InputStream{SessionID: "session-1", Epoch: "epoch-1", NextSequence: math.MaxUint64}
	_, err := stream.Accept(InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: math.MaxUint64})
	if !errors.Is(err, ErrStreamExhausted) {
		t.Fatalf("error = %v, want ErrStreamExhausted", err)
	}
}

func TestInputStreamRejectsUninitializedState(t *testing.T) {
	_, err := (InputStream{}).Accept(InputEnvelope{})
	if !errors.Is(err, ErrInvalidStream) {
		t.Fatalf("error = %v, want ErrInvalidStream", err)
	}
}
