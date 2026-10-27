package inputworker

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peer"
)

func TestUnavailableCursorReplyCannotGrantOtherOperations(t *testing.T) {
	for op := sourceStart; op <= sourceCursorRestore; op++ {
		p := sourcePacket{kind: sourceReply, op: op, status: sourceCursorUnavailable, sequence: 4, reply: 3, binding: Binding{1}}
		data, err := encodeSourcePacket(p)
		if op != sourceCursorPrepare {
			if err == nil {
				t.Fatal("degraded status accepted for input authority", op)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		decoded, err := readSourcePacket(bytes.NewReader(data[:]))
		if err != nil || decoded != p {
			t.Fatal("unavailable snapshot lost in transit", err)
		}
		if got, err := sourceReplyResult(sourcePacket{op: op, sequence: 3}, decoded); err != nil || got.status != sourceCursorUnavailable {
			t.Fatal("optional snapshot became an authority failure", err)
		}
	}
}

func TestSourceBufferedReplySurvivesEOFTimeoutAndCancellation(t *testing.T) {
	request := sourcePacket{kind: sourceCommand, op: sourceAdmit, sequence: 3}
	for _, signal := range []string{"eof", "timeout", "cancel"} {
		for _, status := range []byte{0, 1} {
			ctx, cancel := context.WithCancel(context.Background())
			done := make(chan struct{})
			timeout := make(chan time.Time, 1)
			if signal == "eof" {
				close(done)
			}
			if signal == "timeout" {
				timeout <- time.Now()
			}
			if signal == "cancel" {
				cancel()
			}
			want := sourcePacket{kind: sourceReply, op: sourceAdmit, reply: 3, status: status, decision: peer.BoundaryAdmissionAllowed}
			if status != 0 {
				want.decision = 0
			}
			replies := make(chan sourcePacket, 1)
			replies <- want
			p, known := waitSourceReply(ctx, timeout, replies, done)
			cancel()
			if !known || p != want || len(replies) != 0 {
				t.Fatal("buffered response lost", signal, status, known, p)
			}
			p, err := sourceReplyResult(request, p)
			decision, err := sourceAdmissionResult(p, err)
			if status == 0 && (err != nil || decision != peer.BoundaryAdmissionAllowed) {
				t.Fatal("success lost", err)
			}
			if status != 0 && (decision != 0 || !errors.Is(err, peer.ErrBoundaryInspectionUnavailable)) {
				t.Fatal("rejection became pipe failure", err)
			}
		}
	}
}

func TestSourceBufferedForeignReplyRemainsProtocolFailure(t *testing.T) {
	done := make(chan struct{})
	close(done)
	for _, reply := range []sourcePacket{
		{kind: sourceReply, op: sourceActivate, reply: 3, status: 1},
		{kind: sourceReply, op: sourceAdmit, reply: 4, status: 1},
		{kind: sourceRetired},
	} {
		replies := make(chan sourcePacket, 1)
		replies <- reply
		p, known := waitSourceReply(context.Background(), nil, replies, done)
		if !known {
			t.Fatal("invalid buffered reply discarded")
		}
		p, err := sourceReplyResult(sourcePacket{op: sourceAdmit, sequence: 3}, p)
		_, err = sourceAdmissionResult(p, err)
		if err != ErrProtocol {
			t.Fatal("foreign response became transient rejection", err)
		}
	}
	if _, known := waitSourceReply(context.Background(), nil, make(chan sourcePacket, 1), done); known {
		t.Fatal("EOF invented a response")
	}
}

func TestSourceReplyObservationSeparatesMissingOutcomeFromBufferedReply(t *testing.T) {
	for _, signal := range []string{"cancelled", "timeout", "reader_ended"} {
		for _, buffered := range []bool{false, true} {
			ctx, cancel := context.WithCancel(context.Background())
			done, timeout := make(chan struct{}), make(chan time.Time, 1)
			switch signal {
			case "cancelled":
				cancel()
			case "timeout":
				timeout <- time.Now()
			case "reader_ended":
				close(done)
			}
			replies := make(chan sourcePacket, 1)
			want := sourcePacket{kind: sourceReply, op: sourceAdmit, reply: 1, status: 1}
			if buffered {
				replies <- want
			}
			p, known, outcome := waitSourceReplyObserved(ctx, timeout, replies, done)
			cancel()
			if buffered {
				if !known || p != want || outcome != "received" || len(replies) != 0 {
					t.Fatal("signal erased matching buffered rejection", signal, p, known, outcome)
				}
			} else if known || outcome != signal {
				t.Fatal("missing reply got the wrong terminal observation", signal, known, outcome)
			}
		}
	}
}
