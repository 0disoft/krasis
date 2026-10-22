package inputworker

import (
	"context"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestSourcePresenceTimingSeparatesQueuedAndNativeWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityAgent)
		defer owner.Close()
		entered, release := make(chan struct{}), make(chan struct{})
		var checks atomic.Int32
		owner.authority.checkPresent = func() error {
			if checks.Add(1) == 1 {
				close(entered)
				<-release
			} else {
				time.Sleep(30 * time.Millisecond)
			}
			return nil
		}
		done := make(chan error, 1)
		go func() { done <- owner.authorizeSourcePresence(context.Background()) }()
		<-entered
		time.AfterFunc(200*time.Millisecond, func() { close(release) })
		var profile sourcePresenceTiming
		if err := owner.authorizeSourcePresence(profile.context(context.Background())); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if !profile.measured || profile.wait != 200*time.Millisecond || profile.native != 30*time.Millisecond || checks.Load() != 2 {
			t.Fatal("queued query reused proof or timings mixed", profile, checks.Load())
		}
	})
}

func TestSourcePresenceTimeoutNeverMergesLateNativeWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityAgent)
		defer owner.Close()
		release := make(chan struct{})
		owner.authority.checkPresent = func() error { <-release; return nil }
		var profile sourcePresenceTiming
		ctx, cancel := context.WithTimeout(profile.context(context.Background()), 10*time.Millisecond)
		defer cancel()
		if err := owner.authorizeSourcePresence(ctx); err == nil || !owner.authority.fenced.Load() || owner.Close() != ErrAuthority {
			t.Fatal("timeout lost native owner", err)
		}
		before := profile
		close(release)
		synctest.Wait()
		if profile != before || profile.measured || owner.Close() != nil {
			t.Fatal("late work mutated serial timing or leaked handles", profile)
		}
	})
}
