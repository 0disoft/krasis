package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestClientPresenceQueryCannotOutliveCallerBudget(t *testing.T) {
	for _, heartbeat := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, pipe := net.Pipe()
			defer server.Close()
			stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
			channel := clientChannel(stream, authorityService)
			client, err := NewClient(context.Background(), channel, testLease())
			if err != nil {
				t.Fatal(err)
			}
			release := make(chan struct{})
			var closes, checks atomic.Int32
			channel.authority.closeHandles = func() error { closes.Add(1); return nil }
			channel.authority.checkPresent = func() error {
				checks.Add(1)
				<-release
				if closes.Load() != 0 || stream.closes.Load() != 0 {
					t.Error("presence query lost its live handles")
				}
				return nil
			}
			// Also lets the old synchronous implementation finish and fail the
			// budget assertion, rather than deadlocking the test bubble.
			timer := time.AfterFunc(time.Second, func() { close(release) })
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			started := time.Now()
			if heartbeat {
				err = client.Heartbeat(ctx)
			} else {
				err = client.Move(ctx, 1, 0)
			}
			if time.Since(started) != 50*time.Millisecond || !errors.Is(err, context.DeadlineExceeded) {
				t.Fatalf("presence defeated caller budget: heartbeat=%t elapsed=%s err=%v", heartbeat, time.Since(started), err)
			}
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionNotApplied {
				t.Fatal("presence timeout lost not-sent certainty", err)
			}
			if checks.Load() != 1 || closes.Load() != 0 || stream.closes.Load() != 0 || stream.writes.Load() != 0 || client.sequence != 1 || channel.Close() != ErrAuthority {
				t.Fatal("expired presence query leaked ownership or submitted input")
			}
			if timer.Stop() {
				close(release)
			}
			synctest.Wait()
			if channel.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 {
				t.Fatal("presence owner did not retire after native return")
			}
		})
	}
}

func TestConcurrentPresenceAndInputQueriesRetainHandlesUntilBothReturn(t *testing.T) {
	for _, presenceFirst := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, pipe := net.Pipe()
			defer server.Close()
			stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
			channel := clientChannel(stream, authorityService)
			inputRelease, presenceRelease := make(chan struct{}), make(chan struct{})
			entered := make(chan struct{}, 2)
			var closes atomic.Int32
			channel.authority.closeHandles = func() error { closes.Add(1); return nil }
			channel.authority.checkInput = func() error { entered <- struct{}{}; <-inputRelease; return nil }
			channel.authority.checkPresent = func() error { entered <- struct{}{}; <-presenceRelease; return nil }
			ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
			defer cancel()
			done := make(chan error, 2)
			go func() { done <- channel.authorizeInput(ctx, false) }()
			go func() { done <- channel.authorizePresence(ctx) }()
			<-entered
			<-entered
			for range 2 {
				if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
					t.Fatal("query escaped caller expiry", err)
				}
			}
			first, second := inputRelease, presenceRelease
			if presenceFirst {
				first, second = second, first
			}
			close(first)
			synctest.Wait()
			if closes.Load() != 0 || stream.closes.Load() != 0 || channel.Close() != ErrAuthority {
				t.Fatal("first returning query closed another owner's handles")
			}
			close(second)
			synctest.Wait()
			if channel.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 || len(authorityQueries) != 0 {
				t.Fatal("last returning query did not retire shared resources exactly once")
			}
		})
	}
}
