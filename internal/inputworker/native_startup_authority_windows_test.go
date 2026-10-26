//go:build windows

package inputworker

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"testing/synctest"
	"time"
)

func TestNativeStartupBoundsBothOwnersBeforeProcessCreation(t *testing.T) {
	for _, limit := range []string{"caller", "lease", "default"} {
		t.Run(limit, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				defer isolateAuthorityCapacity(t)()
				aPeer, aPipe := net.Pipe()
				defer aPeer.Close()
				sPeer, sPipe := net.Pipe()
				defer sPeer.Close()
				agent, service := clientChannel(aPipe, authorityAgent), clientChannel(sPipe, authorityService)
				defer agent.Close()
				defer service.Close()
				budget := 50 * time.Millisecond
				ctx := context.Background()
				lease := Lease{ExpiresAt: time.Now().Add(10 * time.Second)}
				switch limit {
				case "caller":
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, budget)
					defer cancel()
				case "lease":
					lease.ExpiresAt = time.Now().Add(budget)
				case "default":
					budget = time.Second
				}
				agent.authority.checkInput = func() error { time.Sleep(3 * budget / 5); return nil }
				service.authority.checkInput = func() error { time.Sleep(4 * budget / 5); return nil }
				// Only satisfy digests(): the test must fail before opening an
				// installed image or creating any native process/handle.
				installation := &Installation{files: make([]*os.File, 5), agentDigest: [32]byte{1}, serviceDigest: [32]byte{2}}
				started := time.Now()
				input, err := startIsolatedInput(ctx, &pipeConn{}, agent, service, lease, installation, nil)
				if input != nil || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != budget {
					t.Fatal("native startup escaped its shared owner budget", time.Since(started), err)
				}
				if !service.authority.fenced.Load() {
					t.Fatal("timed-out service proof still permits input")
				}
				time.Sleep(budget)
				synctest.Wait()
			})
		})
	}
}
