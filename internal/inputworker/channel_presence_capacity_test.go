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

func TestSourceHeartbeatRetainsCapacityUnderFullProofLoad(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		release := make(chan struct{})
		entered := make(chan struct{}, cap(authorityQueries))
		done := make(chan error, cap(authorityQueries))
		var owners []*Channel
		for range cap(authorityQueries) {
			server, pipe := net.Pipe()
			defer server.Close()
			owner := clientChannel(pipe, authorityAgent)
			owners = append(owners, owner)
			owner.authority.checkInput = func() error { entered <- struct{}{}; <-release; return nil }
			go func() { done <- owner.authorizeSourceInput(context.Background()) }()
		}
		defer func() {
			close(release)
			for range owners {
				if err := <-done; err != nil {
					t.Error("full query lost its result", err)
				}
			}
			for _, owner := range owners {
				if err := owner.Close(); err != nil {
					t.Error("full query failed to retire", err)
				}
			}
		}()
		synctest.Wait()
		if len(entered) != cap(authorityQueries)-1 {
			t.Error("full proofs consumed the heartbeat reserve", len(entered))
		}
		var presenceChecks atomic.Int32
		peers := make([]*Channel, 2)
		for i := range peers {
			server, pipe := net.Pipe()
			defer server.Close()
			peers[i] = clientChannel(pipe, authorityAgent)
			defer peers[i].Close()
			peers[i].authority.checkPresent = func() error { presenceChecks.Add(1); return nil }
			peers[i].authority.checkInput = func() error { t.Error("heartbeat requested input authority"); return ErrAuthority }
		}
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer cancel()
		started := time.Now()
		if err := authorizeSourceRead(ctx, peers[0], peers[1], sourceHeartbeat); err != nil || time.Since(started) != 0 || presenceChecks.Load() != 2 {
			t.Error("unrelated full proofs starved fresh service/agent presence", err, time.Since(started), presenceChecks.Load())
		}
		// The medium agent must also be able to preflight an unsent keepalive;
		// otherwise it cannot send the heartbeat that the worker can now read.
		if err := peers[0].authorizeHeartbeatPresence(ctx); err != nil || presenceChecks.Load() != 3 {
			t.Error("full proofs starved unsent heartbeat presence", err)
		}
		// The reserved slot must not authorize capture or create a fifth native
		// query. An unscheduled full proof still expires on the caller's budget.
		full, stopFull := context.WithTimeout(context.Background(), 20*time.Millisecond)
		defer stopFull()
		started = time.Now()
		err := authorizeSourceExecution(full, peers[0], peers[1])
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 20*time.Millisecond || presenceChecks.Load() != 3 || peers[0].authority.fenced.Load() {
			t.Error("full proof bypassed reserve or unsent expiry fenced owner", err)
		}
		waiting := make(chan error, 1)
		go func() { waiting <- peers[0].authorizeSourceInput(context.Background()) }()
		synctest.Wait()
		if err := peers[0].Close(); err != nil {
			t.Error("unscheduled reserve waiter prevented close", err)
		}
		code, _ := reportFailureDetails(<-waiting)
		if reportOperations[code] != "source-query-closed" || len(authorityProofQueries) != cap(authorityProofQueries) {
			t.Error("closed reserve waiter lost owners or failure", reportOperations[code])
		}
	})
}

func TestProofReserveReleasedWhenSharedCapacityWaitEnds(t *testing.T) {
	for _, stop := range []string{"deadline", "close"} {
		t.Run(stop, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				defer isolateAuthorityCapacity(t)()
				_, finish := occupyAuthorityQueries(t)
				defer finish()
				server, pipe := net.Pipe()
				defer server.Close()
				owner := clientChannel(pipe, authorityAgent)
				defer owner.Close()
				var checks atomic.Int32
				owner.authority.checkInput = func() error { checks.Add(1); return nil }
				ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
				defer cancel()
				done := make(chan error, 1)
				go func() { done <- owner.authorizeSourceInput(ctx) }()
				synctest.Wait()
				if len(authorityProofQueries) != 1 {
					t.Fatal("waiting query did not own its restricted reservation")
				}
				if stop == "close" {
					if err := owner.Close(); err != nil {
						t.Fatal("unscheduled caller prevented close", err)
					}
				}
				err := <-done
				code, _ := reportFailureDetails(err)
				if stop == "deadline" && !errors.Is(err, context.DeadlineExceeded) || stop == "close" && reportOperations[code] != "source-query-closed" {
					t.Error("shared-pool wait lost its terminal cause", err)
				}
				if checks.Load() != 0 || len(authorityProofQueries) != 0 || len(authorityQueries) != cap(authorityQueries) || owner.authority.fenced.Load() {
					t.Error("aborted reservation leaked capacity, started work or fenced the unscheduled owner")
				}
			})
		})
	}
}
