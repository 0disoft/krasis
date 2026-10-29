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

func TestSourceWatchdogCannotConsumeSentHeartbeatReserve(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		release := make(chan struct{})
		done := make(chan error, 3)
		var blocked []*Channel
		for range 3 {
			server, pipe := net.Pipe()
			defer server.Close()
			owner := clientChannel(pipe, authorityAgent)
			blocked = append(blocked, owner)
			owner.authority.checkPresent = func() error { <-release; return nil }
			go func() { done <- owner.authorizeSourceWatchdogPresence(context.Background()) }()
		}
		defer func() {
			close(release)
			synctest.Wait()
			for _, owner := range blocked {
				if err := <-done; err != nil {
					t.Error(err)
				}
				if err := owner.Close(); err != nil {
					t.Error(err)
				}
			}
		}()
		synctest.Wait()
		if len(authorityQueries) != 3 || len(authorityProofQueries) != 3 {
			t.Fatal("background presence escaped restricted capacity")
		}
		peers := make([]*Channel, 2)
		var checks atomic.Int32
		for i := range peers {
			server, pipe := net.Pipe()
			defer server.Close()
			peers[i] = clientChannel(pipe, authorityAgent)
			defer peers[i].Close()
			peers[i].authority.checkPresent = func() error { checks.Add(1); return nil }
			peers[i].authority.checkInput = func() error { t.Error("presence granted input authority"); return ErrAuthority }
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		if err := authorizeSourceWatchdog(ctx, peers[0], peers[1]); !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("watchdog consumed heartbeat reserve", err)
		}
		if checks.Load() != 0 || peers[0].authority.fenced.Load() || peers[1].authority.fenced.Load() {
			t.Fatal("unsent watchdog queried or fenced its owner")
		}
		started := time.Now()
		if err := authorizeSourceRead(context.Background(), peers[0], peers[1], sourceHeartbeat); err != nil || checks.Load() != 2 || time.Since(started) != 0 {
			t.Fatal("background monitoring starved sent heartbeat", err, checks.Load())
		}
		if len(authorityQueries) != 3 {
			t.Fatal("heartbeat leaked a query slot")
		}
	})
}

func TestSourceWatchdogSkipsOnlyUnsubmittedCapacityExpiry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		checks := 0
		err := watchSourcePresence(ctx, func() error {
			checks++
			if checks == 1 {
				return sourceQueryOwner("agent", queryCapacityFailure(sourceWatchdogAuthorityQuery, context.DeadlineExceeded))
			}
			cancel()
			return nil
		})
		if err != nil || checks != 2 {
			t.Fatal("unsent capacity expiry killed watcher", err, checks)
		}
	})
	for _, failure := range []error{
		queryPhaseFailure(sourceWatchdogAuthorityQuery, "checking", context.DeadlineExceeded),
		queryPhaseFailure(sourceWatchdogAuthorityQuery, "waiting", context.DeadlineExceeded),
		queryAdmissionFailure(sourceWatchdogAuthorityQuery, "closed", nil),
		queryCapacityFailure(sourceWatchdogAuthorityQuery, context.Canceled),
		errors.Join(queryCapacityFailure(sourceWatchdogAuthorityQuery, context.DeadlineExceeded), errors.New("independent native failure")),
		ErrAuthority,
	} {
		synctest.Test(t, func(t *testing.T) {
			if err := watchSourcePresence(context.Background(), func() error { return failure }); err == nil {
				t.Fatal("watchdog hid submitted, closed, cancelled or native failure", failure)
			}
		})
	}
}

func TestSubmittedWatchdogRetainsNativeOwnershipAfterDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityAgent)
		release := make(chan struct{})
		owner.authority.checkPresent = func() error { <-release; return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		err := owner.authorizeSourceWatchdogPresence(ctx)
		if !errors.Is(err, context.DeadlineExceeded) || !owner.authority.fenced.Load() || owner.Close() != ErrAuthority {
			t.Error("submitted watchdog lost its native owner", err)
		}
		close(release)
		synctest.Wait()
		if err := owner.Close(); err != nil {
			t.Fatal("returned native owner did not retire", err)
		}
	})
}
