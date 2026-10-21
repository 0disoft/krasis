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

func TestSourceCommandNativeChecksRespectCallerBudget(t *testing.T) {
	for _, presence := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, pipe := net.Pipe()
			defer server.Close()
			stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
			channel := clientChannel(stream, authorityService)
			c := &sourceClient{channel: channel, binding: Binding{1}, done: make(chan struct{})}
			release := make(chan struct{})
			var closes atomic.Int32
			channel.authority.closeHandles = func() error { closes.Add(1); return nil }
			native := func() error {
				<-release
				if closes.Load() != 0 || stream.closes.Load() != 0 {
					t.Error("source query lost pinned handles while native work was running")
				}
				return nil
			}
			if presence {
				channel.authority.checkPresent = native
			} else {
				channel.authority.checkInput = native
			}
			timer := time.AfterFunc(time.Second, func() { close(release) })
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			started := time.Now()
			_, err := c.call(ctx, sourceActivate)
			if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond {
				t.Fatalf("source native check defeated deadline: presence=%t elapsed=%s err=%v", presence, time.Since(started), err)
			}
			if c.sequence != 0 || c.epoch.Load() != 0 || stream.writes.Load() != 0 || closes.Load() != 0 || stream.closes.Load() != 0 || channel.Close() != ErrAuthority {
				t.Fatal("expired source query submitted work or lost native ownership")
			}
			if timer.Stop() {
				close(release)
			}
			synctest.Wait()
			if channel.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 {
				t.Fatal("source query did not retire exactly once after native completion")
			}
		})
	}
}

func TestSourceCommandGateWaitRespectsCallerBudget(t *testing.T) {
	for _, op := range []byte{sourceActivate, sourceStop} {
		synctest.Test(t, func(t *testing.T) {
			server, pipe := net.Pipe()
			defer server.Close()
			c := &sourceClient{channel: clientChannel(pipe, authorityService), binding: Binding{1}, done: make(chan struct{})}
			defer c.channel.Close()
			c.mu.Lock()
			timer := time.AfterFunc(time.Second, c.mu.Unlock)
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			started := time.Now()
			_, err := c.call(ctx, op)
			if timer.Stop() {
				c.mu.Unlock()
			}
			if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond || c.commandWaiting.Load() != 0 || c.sequence != 0 || c.epoch.Load() != 0 {
				t.Fatalf("source gate defeated caller budget or leaked demand: op=%d elapsed=%s err=%v", op, time.Since(started), err)
			}
			if c.channel.authority.fenced.Load() || c.channel.authority.closed {
				t.Fatal("expired lock waiter retired an owner without pending native work")
			}
		})
	}
}

func TestSourceHeartbeatAndCommandQueriesRetireAfterEveryNativeOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
		channel := clientChannel(stream, authorityService)
		release := [3]chan struct{}{make(chan struct{}), make(chan struct{}), make(chan struct{})}
		entered := make(chan struct{})
		var presence, closes atomic.Int32
		channel.authority.checkPresent = func() error {
			index := presence.Add(1) - 1
			entered <- struct{}{}
			<-release[index]
			return nil
		}
		channel.authority.checkInput = func() error { entered <- struct{}{}; <-release[2]; return nil }
		channel.authority.closeHandles = func() error { closes.Add(1); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		done := make(chan error, 3)
		go func() { done <- channel.authorizeHeartbeatPresence(ctx) }()
		<-entered
		go func() { done <- channel.authorizePresence(ctx) }()
		<-entered
		go func() { done <- channel.authorizeInput(ctx, false) }()
		<-entered
		for range 3 {
			if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("native owner escaped deadline", err)
			}
		}
		close(release[1])
		close(release[2])
		synctest.Wait()
		if channel.Close() != ErrAuthority || closes.Load() != 0 || stream.closes.Load() != 0 {
			t.Fatal("returning command queries discarded the native heartbeat owner's handles")
		}
		close(release[0])
		synctest.Wait()
		if channel.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 || len(authorityQueries) != 0 {
			t.Fatal("last native owner did not retire all shared resources")
		}
	})
}

func TestExpiredSourceHeartbeatRetainsOneNativeTaskWithoutRenewingOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
		channel := clientChannel(stream, authorityService)
		c := &sourceClient{channel: channel, binding: Binding{1}, done: make(chan struct{})}
		c.epoch.Store(42)
		release := make(chan struct{})
		var checks, closes atomic.Int32
		channel.authority.checkPresent = func() error {
			checks.Add(1)
			<-release
			if closes.Load() != 0 || stream.closes.Load() != 0 {
				t.Error("expired maintenance discarded native handles")
			}
			return nil
		}
		channel.authority.closeHandles = func() error { closes.Add(1); return nil }
		timer := time.AfterFunc(10*time.Second, func() { close(release) })
		for range 2 {
			started := time.Now()
			attempted, err := c.pollHeartbeat(context.Background())
			if attempted || err != nil || time.Since(started) != time.Second {
				t.Fatal("optional unsent query escaped its budget or became a sent failure", attempted, err)
			}
		}
		if checks.Load() != 1 || len(authorityQueries) != 1 || channel.authority.fenced.Load() || c.epoch.Load() != 42 || c.sequence != 0 || stream.writes.Load() != 0 || !c.lastReplyRequestAt.IsZero() {
			t.Fatal("optional queries multiplied, renewed liveness or retired a healthy command owner")
		}
		if channel.Close() != ErrAuthority || closes.Load() != 0 || stream.closes.Load() != 0 {
			t.Fatal("explicit shutdown claimed retirement while native maintenance still owned handles")
		}
		if timer.Stop() {
			close(release)
		}
		synctest.Wait()
		if channel.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 || len(authorityQueries) != 0 {
			t.Fatal("expired maintenance did not finish exactly-once retirement")
		}
	})
}
