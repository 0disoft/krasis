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

func TestPresentationAuthorityHonorsOwnerBudget(t *testing.T) {
	for _, slowService := range []bool{false, true} {
		for _, budget := range []time.Duration{50 * time.Millisecond, time.Second} {
			t.Run(budget.String()+map[bool]string{false: "/agent", true: "/service"}[slowService], func(t *testing.T) {
				synctest.Test(t, func(t *testing.T) {
					defer isolateAuthorityCapacity(t)()
					peer, pipe := net.Pipe()
					defer peer.Close()
					agent := clientChannel(pipe, authorityAgent)
					defer agent.Close()
					servicePeer, servicePipe := net.Pipe()
					defer servicePeer.Close()
					service := clientChannel(servicePipe, authorityService)
					defer service.Close()
					slow := agent
					if slowService {
						slow = service
					}
					var closed atomic.Int32
					slow.authority.closeHandles = func() error { closed.Add(1); return nil }
					slow.authority.checkInput = func() error {
						time.Sleep(2 * budget)
						if closed.Load() != 0 {
							t.Error("presentation released a live native proof")
						}
						return nil
					}
					ctx := context.Background()
					if budget < time.Second {
						var cancel context.CancelFunc
						ctx, cancel = context.WithTimeout(ctx, budget)
						defer cancel()
					}
					started := time.Now()
					err := authorizeCursorPresentation(ctx, agent, service)
					if !errors.Is(err, ErrAuthority) || time.Since(started) != budget {
						t.Errorf("presentation exceeded budget: elapsed=%s err=%v", time.Since(started), err)
					}
					if slow.Close() != ErrAuthority || closed.Load() != 0 {
						t.Error("unfinished native proof lost its owner")
					}
					if errors.Is(retireCursorPresentation(err, agent, service), ErrRecovery) {
						t.Error("query joined within cleanup budget still demanded repair")
					}
					synctest.Wait()
					if slow.Close() != nil || closed.Load() != 1 {
						t.Error("late proof did not retire exactly once")
					}
				})
			})
		}
	}
}

func TestPresentationAuthoritySharesBudgetBetweenBothProofs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		aPeer, aPipe := net.Pipe()
		defer aPeer.Close()
		sPeer, sPipe := net.Pipe()
		defer sPeer.Close()
		agent, service := clientChannel(aPipe, authorityAgent), clientChannel(sPipe, authorityService)
		defer agent.Close()
		defer service.Close()
		agent.authority.checkInput = func() error { time.Sleep(30 * time.Millisecond); return nil }
		service.authority.checkInput = func() error { time.Sleep(40 * time.Millisecond); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := time.Now()
		if err := authorizeCursorPresentation(ctx, agent, service); err == nil || time.Since(started) != 50*time.Millisecond {
			t.Errorf("second proof restarted the budget: elapsed=%s err=%v", time.Since(started), err)
		}
		time.Sleep(100 * time.Millisecond)
		synctest.Wait()
	})
}

func TestPresentationAuthorityAlwaysChecksFreshProofsAndRejectsCanceledOwner(t *testing.T) {
	aPeer, aPipe := net.Pipe()
	defer aPeer.Close()
	sPeer, sPipe := net.Pipe()
	defer sPeer.Close()
	agent, service := clientChannel(aPipe, authorityAgent), clientChannel(sPipe, authorityService)
	defer agent.Close()
	defer service.Close()
	var agentChecks, serviceChecks atomic.Int32
	agent.authority.checkInput = func() error { agentChecks.Add(1); return nil }
	service.authority.checkInput = func() error { serviceChecks.Add(1); return nil }
	for range 2 {
		if err := authorizeCursorPresentation(context.Background(), agent, service); err != nil {
			t.Fatal(err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := authorizeCursorPresentation(ctx, agent, service); !errors.Is(err, context.Canceled) {
		t.Fatal("canceled owner received presentation authority", err)
	}
	if agentChecks.Load() != 2 || serviceChecks.Load() != 2 {
		t.Fatal("proof was cached or canceled request started native work")
	}
	if err := authorizeCursorPresentation(context.Background(), service, agent); !errors.Is(err, ErrAuthority) {
		t.Fatal("swapped owner roles accepted", err)
	}
	if err := retireCursorPresentation(nil, agent, service); err != nil {
		t.Fatal("confirmed retirement became recovery failure", err)
	}
}

func TestPresentationAuthorityRetainsFinalReportDuringPendingProof(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		aPeer, aPipe := net.Pipe()
		defer aPeer.Close()
		agent := clientChannel(aPipe, authorityAgent)
		defer agent.Close()
		peer, pipe := net.Pipe()
		defer peer.Close()
		stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
		proof := clientChannel(stream, authorityService).authority
		lifetime := &workerControlLifetime{stream: stream, proof: proof}
		service := lifetime.captureChannel()
		defer lifetime.Close()
		var closed atomic.Int32
		proof.closeHandles = func() error { closed.Add(1); return nil }
		proof.checkInput = func() error { time.Sleep(3 * time.Second); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		failure := authorizeCursorPresentation(ctx, agent, service)
		retiring := time.Now()
		failure = retireCursorPresentation(failure, agent, service)
		if !errors.Is(failure, ErrRecovery) || time.Since(retiring) != RecoveryTimeout || closed.Load() != 0 || stream.closes.Load() != 0 {
			t.Fatal("pending proof closed its report transport or claimed retirement", failure)
		}
		reported := make(chan error, 1)
		go func() { reported <- finishWorkerReport(stream, Report{Completed: 7}, failure) }()
		report, received := readReport(peer)
		if report.Completed != 7 || !errors.Is(received, ErrRecovery) || !errors.Is(<-reported, ErrRecovery) {
			t.Fatal("lost the independent recovery report", received)
		}
		if err := lifetime.Close(); !errors.Is(err, ErrAuthority) || closed.Load() != 0 || stream.closes.Load() != 0 {
			t.Fatal("final report retired a live native query", err)
		}
		time.Sleep(3 * time.Second)
		synctest.Wait()
		if service.Close() != nil || closed.Load() != 1 || stream.closes.Load() != 1 {
			t.Fatal("native query did not retire its exact handles once")
		}
	})
}

func TestPresentationAuthorityCannotBypassNativeCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		_, finish := occupyAuthorityQueries(t)
		defer finish()
		aPeer, aPipe := net.Pipe()
		defer aPeer.Close()
		sPeer, sPipe := net.Pipe()
		defer sPeer.Close()
		agent, service := clientChannel(aPipe, authorityAgent), clientChannel(sPipe, authorityService)
		defer agent.Close()
		defer service.Close()
		var checks atomic.Int32
		agent.authority.checkInput = func() error { checks.Add(1); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := time.Now()
		err := authorizeCursorPresentation(ctx, agent, service)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond || checks.Load() != 0 {
			t.Fatal("presentation bypassed process-wide native query capacity", err)
		}
	})
}

func TestPresentationAuthorityCancellationInterruptsInFlightProof(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		aPeer, aPipe := net.Pipe()
		defer aPeer.Close()
		sPeer, sPipe := net.Pipe()
		defer sPeer.Close()
		agent, service := clientChannel(aPipe, authorityAgent), clientChannel(sPipe, authorityService)
		defer agent.Close()
		defer service.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		agent.authority.checkInput = func() error {
			cancel()
			time.Sleep(100 * time.Millisecond)
			return nil
		}
		started := time.Now()
		err := authorizeCursorPresentation(ctx, agent, service)
		if !errors.Is(err, context.Canceled) || !time.Now().Equal(started) {
			t.Fatal("presentation cancellation waited for the native proof", err)
		}
		if errors.Is(retireCursorPresentation(err, agent, service), ErrRecovery) || time.Since(started) != 100*time.Millisecond {
			t.Fatal("canceled query joined within budget still demanded repair")
		}
		synctest.Wait()
	})
}
