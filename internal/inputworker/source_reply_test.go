package inputworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/peer"
)

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
