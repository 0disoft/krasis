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

func TestUnsentHeartbeatPreflightHonorsOwnerCancellation(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "cancel", true: "deadline"}[deadline], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				defer isolateAuthorityCapacity(t)()
				_, finish := occupyAuthorityQueries(t)
				defer finish()
				server, pipe := net.Pipe()
				defer server.Close()
				stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
				c := &sourceClient{channel: clientChannel(stream, authorityService), binding: Binding{1}}
				defer c.channel.Close()
				var checks atomic.Int32
				c.channel.authority.checkPresent = func() error { checks.Add(1); return nil }
				ctx, cancel := context.WithCancel(context.Background())
				want := error(context.Canceled)
				if deadline {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), 50*time.Millisecond)
					want = context.DeadlineExceeded
				} else {
					timer := time.AfterFunc(50*time.Millisecond, cancel)
					defer timer.Stop()
				}
				defer cancel()
				started := time.Now()
				attempted, err := c.pollHeartbeat(ctx)
				if attempted || !errors.Is(err, want) || time.Since(started) != 50*time.Millisecond {
					t.Errorf("unsent preflight ignored owner budget: attempted=%t elapsed=%s err=%v", attempted, time.Since(started), err)
				}
				if checks.Load() != 0 || stream.writes.Load() != 0 || c.sequence != 0 || !c.lastReplyRequestAt.IsZero() || c.preflightExpired.Load() || c.channel.authority.fenced.Load() {
					t.Error("canceled unsent maintenance changed authority, wire state or expiry diagnostics")
				}
			})
		})
	}
}

func TestCanceledHeartbeatPreflightRetainsPendingNativeOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		server, pipe := net.Pipe()
		defer server.Close()
		stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
		c := &sourceClient{channel: clientChannel(stream, authorityService), binding: Binding{1}}
		defer c.channel.Close()
		var closes atomic.Int32
		c.channel.authority.closeHandles = func() error { closes.Add(1); return nil }
		c.channel.authority.checkPresent = func() error {
			time.Sleep(200 * time.Millisecond)
			if closes.Load() != 0 || stream.closes.Load() != 0 {
				t.Error("canceled preflight closed handles still owned by native work")
			}
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := time.Now()
		attempted, err := c.pollHeartbeat(ctx)
		if attempted || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond {
			t.Errorf("native preflight defeated caller deadline: elapsed=%s err=%v", time.Since(started), err)
		}
		if c.sequence != 0 || stream.writes.Load() != 0 || c.channel.authority.fenced.Load() || c.channel.Close() != ErrAuthority || closes.Load() != 0 {
			t.Error("unjoined native query was released or cancellation fabricated a sent request")
		}
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		if c.channel.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 {
			t.Error("late native query failed to retire its resources exactly once")
		}
	})
}

func TestSentHeartbeatDrainsReplyAfterOwnerCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		c := &sourceClient{channel: clientChannel(pipe, authorityService), binding: Binding{1}, replies: make(chan sourcePacket, 1), done: make(chan struct{})}
		defer c.channel.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		joined := make(chan struct{})
		go func() {
			defer close(joined)
			p, err := readSourcePacket(server)
			if err != nil {
				t.Error(err)
				return
			}
			cancel() // The request has already crossed the authenticated pipe.
			time.Sleep(100 * time.Millisecond)
			c.replies <- sourcePacket{kind: sourceReply, op: p.op, reply: p.sequence}
		}()
		started := time.Now()
		attempted, err := c.pollHeartbeat(ctx)
		<-joined
		if !attempted || err != nil || c.sequence != 1 || time.Since(started) != 100*time.Millisecond || c.retiring.Load() || c.channel.authority.closed {
			t.Fatal("owner cancellation interrupted a sent heartbeat or lost its matching reply", attempted, err)
		}
	})
}
