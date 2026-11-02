//go:build windows

package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"testing/synctest"
	"time"
)

func TestBoundaryReceiptDoesNotWaitForChildOrCertifyLiveness(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		c.epoch.Store(42)
		previous := time.Now().Add(-sourceHeartbeatQuiet)
		c.lastReplyRequestAt = previous
		frames := make(chan sourcePacket, 2)
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		go func() {
			p, err := readSourcePacket(server)
			if err != nil {
				return
			}
			frames <- p
			// Beyond the old receipt ACK deadline, within unchanged heartbeat budget.
			select {
			case <-time.After(1400 * time.Millisecond):
			case <-ctx.Done():
				return
			}
			p, err = readSourcePacket(server)
			if err == nil {
				frames <- p
				c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
			}
		}()
		started := time.Now()
		if _, err := c.call(context.Background(), sourceBoundaryAck, 16); err != nil {
			t.Fatal("advisory receipt waited for child ACK", err)
		}
		if time.Since(started) != 0 || c.epoch.Load() != 42 || c.lastReplyRequestAt != previous || c.pending.sequence != 0 {
			t.Fatal("receipt delayed submission, changed capture, or fabricated liveness")
		}
		if attempted, err := c.pollHeartbeat(context.Background()); !attempted || err != nil {
			t.Fatal("receipt prevented subsequent exact heartbeat", attempted, err)
		}
		first, second := <-frames, <-frames
		if first.op != sourceBoundaryAck || first.reply != 16 || first.sequence != 1 || second.op != sourceHeartbeat || second.sequence != 2 {
			t.Fatal("receipt replayed or crossed heartbeat sequence", first, second)
		}
	})
}

func TestBoundaryMaintenanceReleasesExactCreditWithoutReply(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	stream := &boundaryBufferedStream{Conn: conn, frames: make(chan []byte, 2)}
	w := &sourceWriter{ctx: context.Background(), channel: clientChannel(stream, authorityAgent), binding: Binding{1}, boundary: newSourceBoundaryWindow()}
	defer w.channel.Close()
	w.started.Store(true)
	r, err := w.boundary.acquire(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	r.sequence.Store(16)
	p := sourcePacket{kind: sourceCommand, op: sourceBoundaryAck, sequence: 1, reply: 15, binding: w.binding}
	if !errors.Is(w.handleSourceMaintenance(p), ErrProtocol) || w.boundary.pending.Load() != r {
		t.Fatal("foreign reference released credit")
	}
	p.reply = 16
	if err := w.handleSourceMaintenance(p); err != nil || w.boundary.pending.Load() != nil || len(stream.frames) != 0 || w.sequence != 0 {
		t.Fatal("receipt wrote an ACK or failed to release credit", err)
	}
	if !errors.Is(w.handleSourceMaintenance(p), ErrProtocol) {
		t.Fatal("duplicate receipt accepted")
	}
	heartbeat := sourcePacket{kind: sourceCommand, op: sourceHeartbeat, sequence: 2, binding: w.binding}
	if err := w.handleSourceMaintenance(heartbeat); err != nil {
		t.Fatal(err)
	}
	reply, err := readSourcePacket(bytes.NewReader(<-stream.frames))
	if err != nil || reply.op != sourceHeartbeat || reply.reply != 2 || reply.sequence != 1 || len(stream.frames) != 0 {
		t.Fatal("heartbeat lost exact reply or found stale receipt ACK", reply, err)
	}
}

func TestBoundaryReceiptAttemptedWriteRemainsFatalWithoutReplay(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	stream := &failingSourceStream{Conn: conn, cause: os.ErrDeadlineExceeded}
	c := &sourceClient{channel: clientChannel(stream, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
	defer c.channel.Close()
	err := sendSourceBoundaryReceipt(context.Background(), 16, func(ctx context.Context, reference uint64) error {
		_, err := c.call(ctx, sourceBoundaryAck, reference)
		return err
	})
	if !errors.Is(err, os.ErrDeadlineExceeded) || stream.writes != 1 || c.sequence != 1 {
		t.Fatal("attempted write hidden or retried", err, stream.writes, c.sequence)
	}
	var retry *sourceReceiptUnsent
	if errors.As(err, &retry) {
		t.Fatal("attempted write marked safe to retry")
	}
}
