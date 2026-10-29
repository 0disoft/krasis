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

func TestSourceWatchdogDoesNotBlockSameChannelHeartbeat(t *testing.T) {
	for _, reject := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			defer isolateAuthorityCapacity(t)()
			server, pipe := net.Pipe()
			defer server.Close()
			owner := clientChannel(pipe, authorityAgent)
			var checks atomic.Int32
			entered, release := make(chan struct{}), make(chan struct{})
			owner.authority.checkPresent = func() error {
				if checks.Add(1) == 1 {
					close(entered)
					<-release
					return nil
				}
				if reject {
					return ErrAuthority
				}
				return nil
			}
			done := make(chan error, 1)
			go func() { done <- owner.authorizeSourceWatchdogPresence(context.Background()) }()
			<-entered
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
			defer cancel()
			started := time.Now()
			err := owner.authorizeSourcePresence(ctx)
			if checks.Load() != 2 || time.Since(started) != 0 || (err != nil) != reject || errors.Is(err, context.DeadlineExceeded) {
				t.Error("sent heartbeat queued behind background query or reused its proof", err, checks.Load(), time.Since(started))
			}
			if len(authorityQueries) != 1 || len(authorityProofQueries) != 1 || owner.authority.fenced.Load() {
				t.Error("heartbeat changed retained watchdog ownership")
			}
			close(release)
			if err := <-done; err != nil {
				t.Error(err)
			}
			synctest.Wait()
			if err := owner.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestSourceWatchdogLaneRetainsHandlesUntilBothQueriesRetire(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityAgent)
		entered := make(chan struct{}, 2)
		releasePresence, releaseWatchdog := make(chan struct{}), make(chan struct{})
		var closes, checks atomic.Int32
		owner.authority.closeHandles = func() error { closes.Add(1); return nil }
		owner.authority.checkPresent = func() error {
			first := checks.Add(1) == 1
			entered <- struct{}{}
			if first {
				<-releaseWatchdog
			} else {
				<-releasePresence
			}
			return nil
		}
		done := make(chan error, 2)
		go func() { done <- owner.authorizeSourceWatchdogPresence(context.Background()) }()
		<-entered
		go func() { done <- owner.authorizeSourcePresence(context.Background()) }()
		synctest.Wait()
		if len(entered) != 1 || len(authorityQueries) != 2 || owner.Close() != ErrAuthority || closes.Load() != 0 {
			t.Error("independent lanes lost a native owner before retirement")
		}
		time.AfterFunc(25*time.Millisecond, func() { close(releasePresence) })
		time.AfterFunc(50*time.Millisecond, func() { close(releaseWatchdog) })
		started := time.Now()
		if err := sourceCaptureQueryRetirement(nil, owner); err != nil || time.Since(started) != 50*time.Millisecond {
			t.Error("retirement omitted the retained watchdog lane", err, time.Since(started))
		}
		for range 2 {
			if err := <-done; !errors.Is(err, ErrAuthority) {
				t.Error("closed owner returned a usable presence proof", err)
			}
		}
		synctest.Wait()
		if owner.Close() != nil || closes.Load() != 1 {
			t.Fatal("query lanes did not close shared handles exactly once", closes.Load())
		}
	})
}
