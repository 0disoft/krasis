package inputworker

import (
	"context"
	"crypto/subtle"
	"errors"
	"io"
	"math"
	"sync/atomic"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
)

// NewPortableClient shares input/cursor validation and decoding with Client,
// but has no Channel/Authority and cannot stand in for a SYSTEM service proof.
// This single-child transport supports fixtures. Product callers must use
// NewPortablePairClient to retain independent parent-death recovery.
func NewPortableClient(ctx context.Context, process *crashguard.Process) (*Client, error) {
	if process == nil {
		return nil, ErrAuthority
	}
	return newPortableClient(ctx, process)
}

// NewPortablePairClient retains the full input/guardian retirement receipt on
// Close. This constructor grants no service or peer authority.
func NewPortablePairClient(ctx context.Context, processes *crashguard.PortableProcesses) (*Client, error) {
	if processes == nil {
		return nil, ErrAuthority
	}
	return newPortableClient(ctx, processes)
}

type portableTransport interface {
	Close() error
	ExchangeInput(context.Context, func(io.Writer, io.Reader) error) error
}

func newPortableClient(ctx context.Context, process portableTransport) (*Client, error) {
	binding, err := NewBinding()
	if err != nil {
		return nil, err
	}
	client := &Client{sequence: 1, localClose: process.Close}
	client.localExchange = func(ctx context.Context, command request, observed func(response)) error {
		return client.performPortable(ctx, process, binding, command, observed)
	}
	// Only local child/desktop readiness; never a peer or lease authorization.
	if err := client.Heartbeat(ctx); err != nil {
		return nil, err
	}
	return client, nil
}

func (client *Client) performPortable(ctx context.Context, process portableTransport, binding Binding, command request, observed func(response)) error {
	if ctx == nil || ctx.Err() != nil {
		return notSent(ErrAuthority)
	}
	ctx, cancel := context.WithTimeout(ctx, IdleTimeout)
	defer cancel()
	if err := client.mu.LockContext(ctx); err != nil {
		return notSent(err)
	}
	defer client.mu.Unlock()
	if client.failed || ctx.Err() != nil || client.sequence == math.MaxUint64 {
		return notSent(ErrExpired)
	}
	command.sequence, command.binding = client.sequence, binding
	data, err := encodeRequest(command)
	if err != nil {
		return notSent(err)
	}
	var submission atomic.Uint32 // 0: unstarted, 1: attempted, 2: permanently fenced.
	var ack response
	err = process.ExchangeInput(ctx, func(writer io.Writer, reader io.Reader) error {
		if ctx.Err() != nil || !submission.CompareAndSwap(0, 1) {
			return ErrExpired
		}
		// Claiming the attempt before Write is conservative. A timeout can
		// instead fence state 0; that late callback then cannot submit input.
		if n, err := writer.Write(data[:]); err != nil || n != len(data) {
			return errors.Join(ErrProtocol, err)
		}
		var err error
		ack, err = readResponse(reader)
		if err != nil || ack.sequence != command.sequence || ack.op != command.op {
			return errors.Join(ErrProtocol, err)
		}
		return nil
	})
	if err != nil {
		client.failed = true
		client.observeFailure("portable_input_failure_unconfirmed")
		if submission.CompareAndSwap(0, 2) || submission.Load() == 2 {
			return notSent(err)
		}
		return unknownInput(err)
	}
	client.sequence++
	if ack.status != 0 {
		client.failed = true
		client.observeFailure("portable_input_failure_acknowledged")
		certainty := application.InjectionOutcomeUnknown
		if ack.certainty == 1 {
			certainty = application.InjectionNotApplied
		} else if ack.certainty == 2 {
			certainty = application.InjectionPartiallyApplied
		}
		return &application.InjectionError{Certainty: certainty, Requested: int(ack.requested), Inserted: int(ack.inserted), Cause: ErrAuthority}
	}
	if observed != nil {
		observed(ack)
	}
	return nil
}

// PortableInputHandler is used only on inherited stdio. Binding and sequence
// identify one local process stream; they never grant peer trust or elevation.
func PortableInputHandler(input application.InputPort, execute func(context.Context, func() error) error) func(io.Reader, io.Writer) error {
	var binding Binding
	sequence := uint64(1)
	var pressed control.PressedState
	failed := false
	return func(reader io.Reader, writer io.Writer) error {
		command, err := readRequest(reader)
		if err != nil || input == nil || execute == nil || command.binding == (Binding{}) || command.sequence != sequence || sequence == math.MaxUint64 || command.op == opClose || command.applyDetail {
			return ErrProtocol
		}
		if binding == (Binding{}) {
			binding = command.binding
		}
		if subtle.ConstantTimeCompare(binding[:], command.binding[:]) != 1 {
			return ErrProtocol
		}
		sequence++
		ack := response{sequence: command.sequence, op: command.op}
		ctx, cancel := context.WithTimeout(context.Background(), IdleTimeout)
		defer cancel()
		var applyErr error
		if failed {
			applyErr = notSent(ErrAuthority)
		} else {
			applyErr = beforeInputCertainty(execute)(ctx, func() error {
				return applyCommand(ctx, input, &pressed, command, &ack)
			})
		}
		if applyErr != nil {
			failed, ack.status = true, 1
			ack.certainty, ack.requested, ack.inserted = describeFailure(applyErr)
		}
		// Keep guard commands available to cancel an unapplied prepared press
		// and to release the independent journal, even after native failure.
		return writeResponse(writer, ack)
	}
}
