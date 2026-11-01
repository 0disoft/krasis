package inputworker

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestBoundaryWindowRequiresExactReceiptAndJoinsOnCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		w := newSourceBoundaryWindow()
		first, err := w.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		first.sequence.Store(16)
		for _, sequence := range []uint64{0, 15, 17} {
			if !errors.Is(w.acknowledge(sequence), ErrProtocol) {
				t.Fatal("foreign receipt admitted", sequence)
			}
		}
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() { _, err := w.acquire(ctx); done <- err }()
		time.Sleep(2 * time.Second)
		select {
		case <-done:
			t.Fatal("unacknowledged batch regained credit")
		default:
		}
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		if err := w.acknowledge(16); err != nil {
			t.Fatal(err)
		}
		if err := w.acknowledge(16); !errors.Is(err, ErrProtocol) {
			t.Fatal("duplicate receipt")
		}
		second, err := w.acquire(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if w.release(first) {
			t.Fatal("old writer released a newer reservation")
		}
		if !w.release(second) {
			t.Fatal("reservation lost")
		}
	})
}

func boundaryReceiptMove() control.InputAction {
	return control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
}

func TestBoundaryReceiptWireCannotGrantInputOrAcknowledgeAnotherSequence(t *testing.T) {
	for _, p := range []sourcePacket{
		{kind: sourceBoundary, sequence: 16, reply: 16, binding: Binding{1}, action: boundaryReceiptMove()},
		{kind: sourceCommand, op: sourceBoundaryAck, sequence: 2, reply: 16, binding: Binding{1}},
		{kind: sourceReply, op: sourceBoundaryAck, sequence: 17, reply: 2, binding: Binding{1}},
	} {
		wire, err := encodeSourcePacket(p)
		if err != nil {
			t.Fatal(err)
		}
		got, err := readSourcePacket(bytes.NewReader(wire[:]))
		if err != nil || got != p {
			t.Fatal(got, err)
		}
		p.reply = 0
		if p.kind == sourceCommand && p.valid() {
			t.Fatal("empty receipt accepted")
		}
	}
	p := sourcePacket{kind: sourceBoundary, sequence: 16, reply: 15, binding: Binding{1}, action: boundaryReceiptMove()}
	if p.valid() {
		t.Fatal("foreign batch terminator")
	}
	full, presence := 0, 0
	if err := sourceCommandReadAuthorization(sourceBoundaryAck, func() error { full++; return ErrAuthority }, func() error { presence++; return nil }); err != nil || full != 0 || presence != 1 {
		t.Fatal("receipt acquired input authority", full, presence, err)
	}
}
