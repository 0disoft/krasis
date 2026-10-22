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

type queryOwnedConn struct {
	*admissionObservedConn
	closes atomic.Int32
}

func (conn *queryOwnedConn) Close() error {
	conn.closes.Add(1)
	return conn.Conn.Close()
}

func TestTimedOutAuthorityQueryRetainsHandlesAndNeverRevives(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
		channel := clientChannel(stream, authorityService)
		client, err := NewClient(context.Background(), channel, testLease())
		if err != nil {
			t.Fatal(err)
		}
		var closes, checks atomic.Int32
		channel.authority.closeHandles = func() error { closes.Add(1); return nil }
		entered, release := make(chan struct{}), make(chan struct{})
		channel.authority.checkInputObserved = func(context.Context) error {
			checks.Add(1)
			close(entered)
			<-release // Deliberately ignore cancellation, like a blocked native RPC.
			if closes.Load() != 0 || stream.closes.Load() != 0 {
				return errors.New("handles closed under native query")
			}
			return nil
		}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- client.Move(ctx, 1, 0) }()
		<-entered
		err = <-done
		if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionNotApplied || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("native RPC defeated caller budget or lost non-submission proof", err)
		}
		if closes.Load() != 0 || stream.closes.Load() != 0 || channel.Close() != ErrAuthority {
			t.Fatal("unconfirmed native query lost ownership or reported cleanup success")
		}
		for range 10 {
			if client.Move(context.Background(), 2, 0) == nil || channel.authority.checkHeartbeat() == nil || channel.authority.checkSource() == nil {
				t.Fatal("retired proof admitted work")
			}
		}
		if checks.Load() != 1 || stream.writes.Load() != 0 || client.sequence != 1 {
			t.Fatal("retired query was replaced or input was replayed")
		}
		close(release)
		synctest.Wait()
		if channel.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 || channel.authority.check(false) == nil {
			t.Fatal("late completion leaked handles or revived the proof")
		}
	})
}

func TestAuthorityQueryCapacityIncludesExpiredNativeOwners(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		release := make(chan struct{})
		entered := make(chan struct{}, cap(authorityQueries))
		done := make(chan error, cap(authorityQueries))
		var checks, closes atomic.Int32
		channels := make([]*Channel, cap(authorityQueries))
		for i := range channels {
			server, pipe := net.Pipe()
			defer server.Close()
			channel := clientChannel(pipe, authorityService)
			channels[i] = channel
			channel.authority.checkInputObserved = func(context.Context) error {
				checks.Add(1)
				entered <- struct{}{}
				<-release
				return nil
			}
			channel.authority.closeHandles = func() error { closes.Add(1); return nil }
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				done <- channel.authorizeInput(ctx, true)
			}()
		}
		for range channels {
			<-entered
		}
		for range channels {
			if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal("expired native query did not return", err)
			}
		}
		server, pipe := net.Pipe()
		defer server.Close()
		replacement := clientChannel(pipe, authorityService)
		replacement.authority.checkInputObserved = func(context.Context) error { t.Error("over-capacity native query started"); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if !errors.Is(replacement.authorizeInput(ctx, true), context.DeadlineExceeded) || len(authorityQueries) != cap(authorityQueries) || closes.Load() != 0 {
			t.Fatal("expired query abandoned its resource slot")
		}
		_ = replacement.Close()
		close(release)
		synctest.Wait()
		if len(authorityQueries) != 0 || int(checks.Load()) != len(channels) || int(closes.Load()) != len(channels) {
			t.Fatal("native query capacity or proof handles leaked")
		}
		server2, pipe2 := net.Pipe()
		defer server2.Close()
		fresh := clientChannel(pipe2, authorityService)
		if fresh.authorizeInput(context.Background(), true) != nil {
			t.Fatal("confirmed retirement did not restore query capacity")
		}
		_ = fresh.Close()
	})
}

func TestClientConstructionHasBoundedAuthorityQuery(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		channel := clientChannel(pipe, authorityService)
		release := make(chan struct{})
		channel.authority.checkInput = func() error { <-release; return nil }
		client, err := NewClient(context.Background(), channel, testLease())
		if client != nil || err != ErrAuthority || !channel.authority.fenced.Load() {
			t.Fatal("client bootstrap outlived its authority budget", err)
		}
		close(release)
		synctest.Wait()
		if channel.Close() != nil {
			t.Fatal("late bootstrap query leaked channel resources")
		}
	})
}
