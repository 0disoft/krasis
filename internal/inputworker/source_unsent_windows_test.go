//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"os"
	"slices"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNativePipeExpiredWriteIsProvenUnsubmittedBeforeTheKernelCall(t *testing.T) {
	for _, read := range []bool{false, true} {
		p := &pipeConn{}
		if p.SetDeadline(time.Now().Add(-time.Second)) != nil {
			t.Fatal("could not configure test deadline")
		}
		called := false
		n, err := p.overlapped(context.Background(), read, []byte{1}, func(*windows.Overlapped) error { called = true; return nil })
		var proof *unsubmittedPipeWrite
		if called || n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) || errors.As(err, &proof) == read {
			t.Fatal("pre-submission expiry lost its direction/proof", read, n, err)
		}
	}
}

type expiryDuringPipeSetup struct {
	context.Context
	pipe  *pipeConn
	reads int
}

func (c *expiryDuringPipeSetup) Err() error {
	c.reads++
	if c.reads == 2 {
		_ = c.pipe.SetWriteDeadline(time.Now().Add(-time.Second))
	}
	return nil
}

func TestNativePipeRechecksExpiryAfterEventAndPinSetupBeforeSubmission(t *testing.T) {
	p := &pipeConn{}
	_ = p.SetWriteDeadline(time.Now().Add(time.Minute))
	ctx := &expiryDuringPipeSetup{Context: context.Background(), pipe: p}
	called := false
	n, err := p.overlapped(ctx, false, []byte{1}, func(*windows.Overlapped) error { called = true; return nil })
	var proof *unsubmittedPipeWrite
	if called || n != 0 || !errors.As(err, &proof) || !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatal("setup delay allowed submission after expiry", called, n, err)
	}
}

func TestSourceHeartbeatProvenUnsubmittedExpiryPreservesOwnerAndSequence(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	stream := &failingSourceStream{Conn: conn, cause: &unsubmittedPipeWrite{cause: os.ErrDeadlineExceeded}}
	c := &sourceClient{channel: clientChannel(stream, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	c.epoch.Store(42)
	var events []string
	c.heartbeatTiming = timingSampler{source: true, emit: func(code string) { events = append(events, code) }}
	attempted, err := c.pollHeartbeat(context.Background())
	if attempted || err != nil || c.sequence != 0 || c.epoch.Load() != 42 || !c.lastReplyRequestAt.IsZero() || c.channel.authority.check(false) != nil {
		t.Fatal("unsent heartbeat fabricated traffic/renewal or fenced the owner", attempted, err, c.sequence)
	}
	if !slices.Contains(events, "installed_source_heartbeat_unsent_expired") {
		t.Fatal("unsent expiry was invisible", events)
	}
	for _, code := range events {
		if strings.Contains(code, "failed") || strings.HasPrefix(code, "installed_source_failure_") {
			t.Fatal("unsubmitted maintenance was logged as a sent failure", events)
		}
	}
	// A later maintenance attempt is a new command with sequence 1. Nothing
	// from the skipped operation was submitted, pending or replayed.
	c.channel.stream = conn
	received := make(chan sourcePacket, 1)
	go func() {
		p, readErr := readSourcePacket(server)
		if readErr == nil {
			received <- p
			c.replies <- sourcePacket{kind: sourceReply, op: sourceHeartbeat, reply: p.sequence}
		}
	}()
	attempted, err = c.pollHeartbeat(context.Background())
	if !attempted || err != nil || c.sequence != 1 || c.lastReplyRequestAt.IsZero() {
		t.Fatal("fresh maintenance could not resume", attempted, err)
	}
	select {
	case p := <-received:
		if p.sequence != 1 || p.op != sourceHeartbeat {
			t.Fatal("unsent operation consumed a wire sequence", p.sequence)
		}
	default:
		t.Fatal("maintenance reply had no matching received request")
	}
}

func TestSourceHeartbeatUnknownOrPartialWriteStillFailsWithoutReplay(t *testing.T) {
	for _, tc := range []struct {
		name    string
		op      byte
		cause   error
		partial bool
	}{
		{"unknown_zero", sourceHeartbeat, os.ErrDeadlineExceeded, false},
		{"partial", sourceHeartbeat, os.ErrDeadlineExceeded, true},
		{"contradictory_partial_proof", sourceHeartbeat, &unsubmittedPipeWrite{cause: os.ErrDeadlineExceeded}, true},
		{"ordinary_command", sourceActivate, &unsubmittedPipeWrite{cause: os.ErrDeadlineExceeded}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, conn := net.Pipe()
			defer server.Close()
			stream := &failingSourceStream{Conn: conn, cause: tc.cause, partial: tc.partial}
			c := &sourceClient{channel: clientChannel(stream, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
			defer c.channel.Close()
			c.epoch.Store(42)
			_, err := c.call(context.Background(), tc.op)
			if err == nil || c.epoch.Load() != 0 || c.sequence != 1 || stream.writes != 1 || c.channel.authority.check(false) == nil {
				t.Fatal("uncertain/ordinary failure was skipped or retried", err, c.sequence, stream.writes)
			}
		})
	}
}
