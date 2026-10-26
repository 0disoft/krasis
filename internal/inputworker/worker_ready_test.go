package inputworker

import (
	"context"
	"errors"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestWorkerReadyProofAndWriteShareRemainingBudget(t *testing.T) {
	for _, limit := range []string{"caller", "lease", "default"} {
		t.Run(limit, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				peer, pipe := net.Pipe()
				defer peer.Close()
				service := clientChannel(pipe, authorityService)
				defer service.Close()
				budget, expires := 50*time.Millisecond, time.Now().Add(10*time.Second)
				ctx := context.Background()
				switch limit {
				case "caller":
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, budget)
					defer cancel()
				case "lease":
					expires = time.Now().Add(budget)
				case "default":
					budget = time.Second
				}
				service.authority.checkInput = func() error { time.Sleep(budget / 2); return nil }
				started := time.Now()
				err := publishWorkerReady(ctx, service, pipe, expires, handoff.StartupProfile{})
				if !errors.Is(err, ErrAuthority) || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != budget {
					t.Fatal("READY write restarted the authorization budget", time.Since(started), err)
				}
			})
		})
	}
}

func TestWorkerReadyPendingProofRetainsHandlesAndFinalReport(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		peer, pipe := net.Pipe()
		defer peer.Close()
		stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
		proof := clientChannel(stream, authorityService).authority
		lifetime := &workerControlLifetime{stream: stream, proof: proof}
		service := lifetime.captureChannel()
		defer lifetime.Close()
		var closes atomic.Int32
		proof.closeHandles = func() error { closes.Add(1); return nil }
		proof.checkInput = func() error { time.Sleep(2 * time.Second); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		failure := publishWorkerReady(ctx, service, stream, time.Now().Add(time.Minute), handoff.StartupProfile{})
		if !errors.Is(failure, context.DeadlineExceeded) || service.Close() != ErrAuthority || closes.Load() != 0 || stream.closes.Load() != 0 {
			t.Fatal("READY retired a live proof or hid uncertain cleanup", failure)
		}
		failure = errors.Join(failure, serviceRecoveryFailure(nil, service.Close()))
		done := make(chan error, 1)
		go func() { done <- finishWorkerReport(stream, Report{Completed: 7}, failure) }()
		report, received := readReport(peer)
		if report.Completed != 7 || !errors.Is(received, ErrRecovery) || !errors.Is(<-done, ErrRecovery) {
			t.Fatal("pending READY proof lost the independent final report", received)
		}
		_ = lifetime.Close()
		if closes.Load() != 0 || stream.closes.Load() != 0 {
			t.Fatal("final report closed handles owned by a live query")
		}
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if service.Close() != nil || closes.Load() != 1 || stream.closes.Load() != 1 {
			t.Fatal("late proof failed exact retirement")
		}
	})
}

func TestWorkerReadySuccessDisarmsCancellationAndPreservesProfile(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer, pipe := net.Pipe()
		defer peer.Close()
		service := clientChannel(pipe, authorityService)
		defer service.Close()
		var checks atomic.Int32
		service.authority.checkInput = func() error { checks.Add(1); return nil }
		var profile handoff.StartupProfile
		profile.Record(handoff.WorkerDesktopReady, time.Millisecond)
		done := make(chan error, 1)
		go func() {
			got, err := readStartupReady(peer)
			if err == nil && (got == nil || *got != profile) {
				err = ErrProtocol
			}
			done <- err
		}()
		ctx, cancel := context.WithCancel(context.Background())
		if err := publishWorkerReady(ctx, service, pipe, time.Now().Add(time.Minute), profile); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		cancel()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		go func() { done <- finishWorkerReport(pipe, Report{Completed: 9}, nil) }()
		report, received := readReport(peer)
		if report.Completed != 9 || received != nil || <-done != nil || checks.Load() != 1 || service.authority.fenced.Load() {
			t.Fatal("completed READY retained cancellation or skipped its fresh proof", received)
		}
	})
}

func TestWorkerReadyCancellationInterruptsBlockedWrite(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer, pipe := net.Pipe()
		defer peer.Close()
		service := clientChannel(pipe, authorityService)
		defer service.Close()
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan error, 1)
		go func() {
			done <- publishWorkerReady(ctx, service, pipe, time.Now().Add(time.Minute), handoff.StartupProfile{})
		}()
		synctest.Wait()
		started := time.Now()
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) || !time.Now().Equal(started) {
			t.Fatal("cancellation waited for the READY write deadline", err)
		}
	})
}
