package control

import (
	"errors"
	"fmt"
	"math"
)

var ErrInvalidOutboundStream = errors.New("invalid outbound input stream")

type OutboundStream struct {
	SessionID    string
	Epoch        string
	NextSequence uint64
}

func NewOutboundStream(sessionID, epoch string) (OutboundStream, error) {
	if sessionID == "" || epoch == "" {
		return OutboundStream{}, fmt.Errorf("%w: session and epoch are required", ErrInvalidOutboundStream)
	}
	return OutboundStream{SessionID: sessionID, Epoch: epoch, NextSequence: 1}, nil
}

func (stream OutboundStream) Next() (InputEnvelope, OutboundStream, error) {
	if stream.SessionID == "" || stream.Epoch == "" || stream.NextSequence == 0 {
		return InputEnvelope{}, OutboundStream{}, fmt.Errorf("%w: stream is not initialized", ErrInvalidOutboundStream)
	}
	if stream.NextSequence == math.MaxUint64 {
		return InputEnvelope{}, OutboundStream{}, ErrStreamExhausted
	}
	envelope := InputEnvelope{
		SessionID: stream.SessionID,
		Epoch:     stream.Epoch,
		Sequence:  stream.NextSequence,
	}
	next := stream
	next.NextSequence++
	return envelope, next, nil
}
