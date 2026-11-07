package inputworker

import (
	"context"
	"errors"
	"net"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

func TestActivationAuthorityTimingJoinsFreshQueriesAndIgnoresLateResults(t *testing.T) {
	for _, expired := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, pipe := net.Pipe()
			defer server.Close()
			owner := clientChannel(pipe, authorityService)
			defer owner.Close()
			owner.authority.checkInput = func() error { time.Sleep(30 * time.Millisecond); return nil }
			owner.authority.checkPresent = func() error { time.Sleep(5 * time.Millisecond); return nil }
			var profile sourcePresenceTiming
			ctx := profile.context(context.WithValue(context.Background(), sourceCommandAuthorityKey{}, true))
			if expired {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 10*time.Millisecond)
				defer cancel()
			}
			err := owner.authorizeInput(ctx, false)
			if expired {
				if !errors.Is(err, context.DeadlineExceeded) || profile.measured {
					t.Fatal("expired native query was merged", err, profile)
				}
				time.Sleep(20 * time.Millisecond)
				synctest.Wait()
				if profile.detail().Measured || owner.Close() != nil {
					t.Fatal("late query changed profile or retained handles", profile)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if err := owner.authorizePresence(ctx); err != nil {
				t.Fatal(err)
			}
			d := profile.detail()
			if !d.Measured || d.NativeMicros != 35000 || d.WaitMicros != 0 || !d.Valid() {
				t.Fatal("native work mixed with scheduling", d)
			}
		})
	}
}

func TestActivationReadTimingChecksBothFreshOwnersAndPreservesError(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		serverA, pipeA := net.Pipe()
		serverB, pipeB := net.Pipe()
		defer serverA.Close()
		defer serverB.Close()
		service, agent := clientChannel(pipeA, authorityService), clientChannel(pipeB, authorityAgent)
		defer service.Close()
		defer agent.Close()
		var checks int
		service.authority.checkPresent = func() error { checks++; time.Sleep(7 * time.Millisecond); return nil }
		failure := authorityCheckFailed("authority-process-query", syscall.Errno(5))
		agent.authority.checkPresent = func() error { checks++; time.Sleep(3 * time.Millisecond); return failure }
		for range 2 {
			var p sourcePresenceTiming
			err := authorizeSourceRead(p.context(context.Background()), service, agent, sourceActivate)
			var native *authorityCheckFailure
			if !errors.As(err, &native) || native.operation != "authority-process-query" || native.native != 5 || !p.measured || p.native != 10*time.Millisecond || p.wait != 0 {
				t.Fatal("fresh owner proof or native cause lost", err, p)
			}
		}
		if checks != 4 {
			t.Fatal("timing reused earlier proof", checks)
		}
	})
}
