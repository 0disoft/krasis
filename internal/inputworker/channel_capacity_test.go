package inputworker

import (
	"context"
	"errors"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

// A blocked select on a process-global channel created outside a synctest
// bubble cannot advance fake time. All tests here are serial and join their
// owners before restoring the process-wide semaphore.
func isolateAuthorityCapacity(t *testing.T) func() {
	t.Helper()
	previous := authorityQueries
	previousProofs := authorityProofQueries
	if len(previous) != 0 || len(previousProofs) != 0 {
		t.Fatal("preceding native query still owns global capacity")
	}
	authorityQueries = make(chan struct{}, cap(previous))
	authorityProofQueries = make(chan struct{}, cap(previousProofs))
	return func() {
		// A published query result precedes its deferred slot release. Join
		// that final goroutine work before restoring the shared test fixture.
		synctest.Wait()
		if len(authorityQueries) != 0 || len(authorityProofQueries) != 0 {
			t.Error("test left an unretired capacity owner")
		}
		authorityQueries = previous
		authorityProofQueries = previousProofs
	}
}

// Own all four slots with real retained query goroutines, not synthetic tokens.
// The caller can release one owner while the others remain in native work.
func occupyAuthorityQueries(t *testing.T) (releaseOne func(), finish func()) {
	t.Helper()
	if len(authorityQueries) != 0 {
		t.Fatal("query capacity was not retired by the preceding test")
	}
	first, all := make(chan struct{}), make(chan struct{})
	var firstOnce, allOnce sync.Once
	entered := make(chan struct{}, cap(authorityQueries))
	done := make(chan error, cap(authorityQueries))
	var owners []*Channel
	for i := range cap(authorityQueries) {
		server, pipe := net.Pipe()
		t.Cleanup(func() { _ = server.Close() })
		owner := clientChannel(pipe, authorityAgent)
		owners = append(owners, owner)
		owner.authority.checkPresent = func() error {
			entered <- struct{}{}
			if i == 0 {
				select {
				case <-first:
				case <-all:
				}
			} else {
				<-all
			}
			return nil
		}
		go func() { done <- owner.authorizeSourcePresence(context.Background()) }()
	}
	for range owners {
		<-entered
	}
	return func() { firstOnce.Do(func() { close(first) }) }, func() {
		allOnce.Do(func() { close(all) })
		for range owners {
			if err := <-done; err != nil {
				t.Error("capacity fixture lost a native owner", err)
			}
		}
		for _, owner := range owners {
			if err := owner.Close(); err != nil {
				t.Error("capacity fixture did not retire", err)
			}
		}
		synctest.Wait()
		if len(authorityQueries) != 0 {
			t.Error("query slots leaked after native owners returned")
		}
	}
}

func TestSourceAuthorityTransientCapacityWaitsForFreshProof(t *testing.T) {
	for _, reject := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			defer isolateAuthorityCapacity(t)()
			release, finish := occupyAuthorityQueries(t)
			defer finish()
			server, pipe := net.Pipe()
			defer server.Close()
			owner := clientChannel(pipe, authorityAgent)
			defer owner.Close()
			var checks atomic.Int32
			owner.authority.checkPresent = func() error {
				checks.Add(1)
				if reject {
					return ErrAuthority
				}
				return nil
			}
			timer := time.AfterFunc(50*time.Millisecond, release)
			defer timer.Stop()
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()
			started := time.Now()
			err := owner.authorizeSourcePresence(ctx)
			if time.Since(started) != 50*time.Millisecond || checks.Load() != 1 || owner.authority.fenced.Load() || (err != nil) != reject {
				t.Fatal("temporary query load rejected a live owner or bypassed fresh authority", err, time.Since(started), checks.Load())
			}
		})
	}
}

func TestSourceAuthorityCapacityWaitUsesCallerBudgetWithoutNativeWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		_, finish := occupyAuthorityQueries(t)
		defer finish()
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityAgent)
		defer owner.Close()
		var checks atomic.Int32
		owner.authority.checkPresent = func() error { checks.Add(1); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		started := time.Now()
		err := owner.authorizeSourcePresence(ctx)
		code, _ := reportFailureDetails(err)
		if !errors.Is(err, context.DeadlineExceeded) || !errors.Is(err, ErrAuthority) || reportOperations[code] != "source-query-capacity" || time.Since(started) != 20*time.Millisecond {
			t.Fatal("capacity admission renewed or lost the caller budget", err, reportOperations[code], time.Since(started))
		}
		if checks.Load() != 0 || owner.authority.fenced.Load() || len(authorityQueries) != cap(authorityQueries) || owner.Close() != nil {
			t.Fatal("unscheduled caller started native work or lost retained owners")
		}
	})
}

func TestSourceAuthorityCloseReleasesCapacityWaiter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		_, finish := occupyAuthorityQueries(t)
		defer finish()
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityAgent)
		defer owner.Close()
		var checks atomic.Int32
		owner.authority.checkPresent = func() error { checks.Add(1); return nil }
		done := make(chan error, 1)
		go func() { done <- owner.authorizeSourcePresence(context.Background()) }()
		synctest.Wait()
		if err := owner.Close(); err != nil {
			t.Fatal("unscheduled caller prevented channel retirement", err)
		}
		code, _ := reportFailureDetails(<-done)
		if reportOperations[code] != "source-query-closed" || checks.Load() != 0 || len(authorityQueries) != cap(authorityQueries) {
			t.Fatal("closed waiter started work or retained the wrong failure", reportOperations[code], checks.Load())
		}
	})
}

func TestSourceAuthorityCapacityWaitersRecheckSharedLane(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		for range cap(authorityQueries) {
			authorityQueries <- struct{}{}
		}
		server, pipe := net.Pipe()
		defer server.Close()
		owner := clientChannel(pipe, authorityAgent)
		defer owner.Close()
		entered, release := make(chan struct{}), make(chan struct{})
		var once sync.Once
		defer once.Do(func() { close(release) })
		var checks atomic.Int32
		owner.authority.checkPresent = func() error {
			if checks.Add(1) == 1 {
				close(entered)
				<-release
				return nil
			}
			return ErrAuthority // Revocation must be seen by the next waiter.
		}
		done := make(chan error, 2)
		for range 2 {
			go func() { done <- owner.authorizeSourcePresence(context.Background()) }()
		}
		synctest.Wait()
		// Both callers saw an empty lane before capacity admission. Free two
		// slots at once; only one may publish native work for this lane.
		<-authorityQueries
		<-authorityQueries
		<-entered
		synctest.Wait()
		if checks.Load() != 1 {
			t.Error("capacity reservation bypassed the shared lane")
		}
		once.Do(func() { close(release) })
		first, second := <-done, <-done
		synctest.Wait()
		if checks.Load() != 2 || (first == nil) == (second == nil) || first != nil && !errors.Is(first, ErrAuthority) || second != nil && !errors.Is(second, ErrAuthority) {
			t.Error("waiters reused an earlier proof or lost later revocation", first, second, checks.Load())
		}
		// The two unreleased fixture reservations are still owned.
		if len(authorityQueries) != cap(authorityQueries)-2 {
			t.Error("lane race leaked a capacity reservation")
		}
		for range cap(authorityQueries) - 2 {
			<-authorityQueries
		}
	})
}
