//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestBoundaryReceiptSurvivesUnsentPresenceStallWithoutFencing(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		var checks atomic.Int32
		c.channel.authority.checkPresent = func() error {
			if checks.Add(1) == 1 {
				time.Sleep(1400 * time.Millisecond)
			}
			return nil
		}
		got := make(chan sourcePacket, 1)
		go func() {
			p, err := readSourcePacket(server)
			if err == nil {
				got <- p
				c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
			}
		}()
		start := time.Now()
		err := sendSourceBoundaryReceipt(context.Background(), 16, func(ctx context.Context, ref uint64) error {
			_, err := c.call(ctx, sourceBoundaryAck, ref)
			return err
		})
		if err != nil || c.channel.authority.fenced.Load() || c.sequence != 1 || c.epoch.Load() != 0 || checks.Load() != 2 {
			t.Fatal("unsent stall retired owner, reused proof, or added redundant query", err, checks.Load(), c.sequence)
		}
		if elapsed := time.Since(start); elapsed < 1400*time.Millisecond || elapsed >= IdleTimeout {
			t.Fatal("receipt missed remaining owner budget", elapsed)
		}
		if p := <-got; p.op != sourceBoundaryAck || p.reply != 16 || p.sequence != 1 {
			t.Fatal("receipt replayed or changed reference", p)
		}
	})
}

func TestBoundaryReceiptNativeWaitLeavesHeartbeatRunnable(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, conn := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(conn, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		var checks atomic.Int32
		c.channel.authority.checkPresent = func() error {
			if checks.Add(1) == 1 {
				time.Sleep(1400 * time.Millisecond)
			}
			return nil
		}
		frames := make(chan sourcePacket, 2)
		go func() {
			for range 2 {
				p, err := readSourcePacket(server)
				if err != nil {
					return
				}
				frames <- p
				c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
			}
		}()
		heartbeat := make(chan error, 1)
		go func() {
			time.Sleep(100 * time.Millisecond)
			attempted, err := c.pollHeartbeat(context.Background())
			if !attempted && err == nil {
				err = ErrProtocol
			}
			heartbeat <- err
		}()
		err := sendSourceBoundaryReceipt(context.Background(), 16, func(ctx context.Context, ref uint64) error { _, err := c.call(ctx, sourceBoundaryAck, ref); return err })
		if hb := <-heartbeat; hb != nil || err != nil || c.channel.authority.fenced.Load() {
			t.Fatal("receipt query blocked heartbeat", hb, err)
		}
		first, second := <-frames, <-frames
		if first.op != sourceHeartbeat || first.sequence != 1 || second.op != sourceBoundaryAck || second.sequence != 2 || second.reply != 16 || checks.Load() != 3 {
			t.Fatal("receipt changed independent heartbeat/order/fresh-proof behavior", first, second, checks.Load())
		}
	})
}

func TestBoundaryReceiptQueryRetainsHandlesUntilNativeReturn(t *testing.T) {
	server, conn := net.Pipe()
	defer server.Close()
	channel := clientChannel(conn, authorityService)
	entered, release := make(chan struct{}), make(chan struct{})
	var closes atomic.Int32
	channel.authority.checkPresent = func() error { close(entered); <-release; return nil }
	channel.authority.closeHandles = func() error { closes.Add(1); return nil }
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- channel.authorizeReceiptPresence(ctx) }()
	<-entered
	channel.queryMu.Lock()
	query := channel.receiptQuery
	channel.queryMu.Unlock()
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) || channel.authority.fenced.Load() {
		t.Fatal("unsent receipt cancellation fenced input", err)
	}
	if channel.Close() != ErrAuthority || closes.Load() != 0 {
		t.Fatal("pending receipt falsely certified native cleanup")
	}
	close(release)
	<-query.done
	if channel.Close() != nil || closes.Load() != 1 {
		t.Fatal("receipt did not retire handles exactly once")
	}
}
