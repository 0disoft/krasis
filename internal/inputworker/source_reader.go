package inputworker

import (
	"context"
	"io"
)

type sourceContextReader struct {
	ctx  context.Context
	read func(context.Context, []byte) (int, error)
}

func (r sourceContextReader) Read(data []byte) (int, error) { return r.read(r.ctx, data) }

// The concrete pipe can return the local read context's cancellation only for
// its own interrupted overlapped read. Do not replace external EOF/timeout or
// a validation/authority failure with that context just because stop raced it.
func readSourceCommand(reader io.Reader, local context.Context, binding Binding, previous uint64, authorize func() error) (sourcePacket, error) {
	p, err := readSourcePacket(reader)
	if err != nil {
		if err == context.Canceled && local.Err() == context.Canceled {
			return sourcePacket{}, context.Canceled
		}
		return sourcePacket{}, sourceTransportFailure(err)
	}
	if p.kind != sourceCommand || p.binding != binding || p.sequence != previous+1 {
		return sourcePacket{}, ErrProtocol
	}
	if err := authorize(); err != nil {
		return sourcePacket{}, err
	}
	return p, nil
}

// Its terminal result is joined independently of cancellation of the worker.
// A read/authentication/validation failure therefore cannot become success.
func pumpSourceCommands(run context.Context, read func() (sourcePacket, error), requests chan<- sourcePacket, cancel context.CancelFunc) error {
	for {
		p, err := read()
		if err != nil {
			cancel()
			return err
		}
		select {
		case requests <- p:
		case <-run.Done():
			return run.Err()
		}
		if p.op == sourceStop {
			return nil
		}
	}
}
