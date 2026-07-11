package control

import (
	"errors"
	"fmt"
	"math"
)

var (
	ErrInvalidStream    = errors.New("invalid input stream")
	ErrStaleSession     = errors.New("stale input session")
	ErrStaleEpoch       = errors.New("stale input epoch")
	ErrDuplicateInput   = errors.New("duplicate input sequence")
	ErrInputSequenceGap = errors.New("input sequence gap")
	ErrStreamExhausted  = errors.New("input stream sequence exhausted")
)

type InputStream struct {
	SessionID    string
	Epoch        string
	NextSequence uint64
}

type InputEnvelope struct {
	SessionID string
	Epoch     string
	Sequence  uint64
}

func NewInputStream(sessionID, epoch string) (InputStream, error) {
	if sessionID == "" || epoch == "" {
		return InputStream{}, fmt.Errorf("%w: session and epoch are required", ErrInvalidStream)
	}
	return InputStream{SessionID: sessionID, Epoch: epoch, NextSequence: 1}, nil
}

func (stream InputStream) Accept(envelope InputEnvelope) (InputStream, error) {
	if stream.SessionID == "" || stream.Epoch == "" || stream.NextSequence == 0 {
		return InputStream{}, fmt.Errorf("%w: stream is not initialized", ErrInvalidStream)
	}
	if envelope.SessionID != stream.SessionID {
		return InputStream{}, fmt.Errorf("%w: envelope session does not match", ErrStaleSession)
	}
	if envelope.Epoch != stream.Epoch {
		return InputStream{}, fmt.Errorf("%w: envelope epoch does not match", ErrStaleEpoch)
	}
	if envelope.Sequence < stream.NextSequence {
		return InputStream{}, fmt.Errorf("%w: got=%d want=%d", ErrDuplicateInput, envelope.Sequence, stream.NextSequence)
	}
	if envelope.Sequence > stream.NextSequence {
		return InputStream{}, fmt.Errorf("%w: got=%d want=%d", ErrInputSequenceGap, envelope.Sequence, stream.NextSequence)
	}
	if stream.NextSequence == math.MaxUint64 {
		return InputStream{}, ErrStreamExhausted
	}
	next := stream
	next.NextSequence++
	return next, nil
}
