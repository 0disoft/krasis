//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

func TestOptionalSourceTimeoutDrainsExactReplyBeforeCleanStop(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{}), failed: make(chan struct{})}
	defer c.channel.Close()
	c.epoch.Store(42)
	sent, release, stopped := make(chan struct{}), make(chan struct{}), make(chan struct{})
	go func() {
		p, err := readSourcePacket(server)
		if err != nil {
			return
		}
		close(sent)
		<-release
		c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence, status: 1}
		stop, err := readSourcePacket(server)
		if err == nil && stop.op == sourceStop && stop.sequence == p.sequence+1 {
			c.replies <- sourcePacket{kind: sourceReply, op: sourceStop, reply: stop.sequence}
			close(stopped)
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	if _, err := c.call(ctx, sourceCursorPrepare); !errors.Is(err, ErrAuthority) {
		t.Fatal("optional timeout unexpectedly succeeded", err)
	}
	<-sent
	if !c.retiring.Load() || c.epoch.Load() != 0 || c.pending.sequence != 1 || c.channel.authority.check(false) != nil {
		t.Fatal("timed out request closed pipe or kept capture authority")
	}
	select {
	case <-c.failed:
	default:
		t.Fatal("owner was not notified")
	}
	if _, err := c.call(context.Background(), sourceActivate); err == nil || c.sequence != 1 {
		t.Fatal("new input command was sent while draining")
	}
	close(release)
	cleanup, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, err := c.call(cleanup, sourceStop); err != nil {
		t.Fatal("exact late rejection prevented clean shutdown", err)
	}
	<-stopped
	if c.pending.sequence != 0 || c.sequence != 2 {
		t.Fatal("request replayed or not drained")
	}
}

func TestSourcePendingReplyCannotInventRetirementProof(t *testing.T) {
	for _, tc := range []struct {
		name  string
		reply sourcePacket
		want  error
	}{
		{"wrong sequence", sourcePacket{op: sourceCursorShow, reply: 9}, ErrProtocol},
		{"wrong operation", sourcePacket{op: sourceActivate, reply: 1}, ErrProtocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := &sourceClient{pending: sourcePacket{op: sourceCursorShow, sequence: 1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
			c.replies <- tc.reply
			if err := c.drainPending(context.Background()); err != tc.want || c.pending.sequence == 0 {
				t.Fatal("foreign reply accepted", err)
			}
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := &sourceClient{pending: sourcePacket{op: sourceCursorShow, sequence: 1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	if err := c.drainPending(ctx); err == nil || c.pending.sequence == 0 {
		t.Fatal("missing receipt accepted")
	}
	for _, op := range []byte{sourceStart, sourceAdmit, sourceActivate, sourceDeactivate, sourceHeartbeat, sourceStop} {
		if optionalSourceCursor(op) {
			t.Fatal("input or control was classified optional", op)
		}
	}
}
