package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestNativeExecutionUsesOneFreshDesktopAgentCheck(t *testing.T) {
	for _, denied := range []string{"", "service", "agent", "parent"} {
		t.Run(denied, func(t *testing.T) {
			var serviceChecks atomic.Int32
			agentChecks, applied := 0, 0
			execute := nativeInputExecutor(func(_ context.Context, work func() error) error {
				agentChecks++
				if denied == "agent" {
					return ErrAuthority
				}
				return work()
			}, func() error {
				serviceChecks.Add(1)
				if denied == "service" {
					return ErrAuthority
				}
				return nil
			}, func() bool { return denied != "parent" }, time.Now().Add(time.Second), false)
			err := execute(context.Background(), func() error { applied++; return nil })
			if (err == nil) != (denied == "") || (applied == 1) != (denied == "") || serviceChecks.Load() != 1 || agentChecks < 1 || agentChecks > 2 {
				t.Fatal("duplicate service proof or denied native execution", err, serviceChecks.Load(), agentChecks, applied)
			}
		})
	}
}

func TestNativeExecutionOverlapsFreshProofsWithoutReplayingInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		var checks atomic.Int32
		applied := 0
		var timing workerTiming
		execute := nativeInputExecutor(func(_ context.Context, work func() error) error {
			time.Sleep(24 * time.Millisecond)
			return work()
		}, func() error {
			checks.Add(1)
			time.Sleep(10 * time.Millisecond)
			return nil
		}, func() bool { return true }, time.Now().Add(time.Second), false, &timing)
		if err := execute(context.Background(), func() error {
			applied++
			time.Sleep(3 * time.Millisecond)
			return nil
		}); err != nil || applied != 1 || checks.Load() != 1 || time.Since(started) != 27*time.Millisecond {
			t.Fatal("independent proofs serialized or input replayed", err, applied, time.Since(started))
		}
		if timing.millis != [3]uint16{10, 24, 3} || timing.measured != 7 {
			t.Fatal("overlapping proof/native timings lost their boundaries", timing)
		}
	})
}

func TestNativeExecutionReentersDesktopAfterLateServiceProof(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			var serviceDone atomic.Bool
			checks, applied := 0, 0
			execute := nativeInputExecutor(func(_ context.Context, work func() error) error {
				checks++
				if checks == 1 {
					time.Sleep(5 * time.Millisecond)
				} else if !serviceDone.Load() {
					t.Fatal("desktop rechecked before service completion")
				} else if revoke {
					return ErrAuthority
				}
				return work()
			}, func() error {
				time.Sleep(20 * time.Millisecond)
				serviceDone.Store(true)
				return nil
			}, func() bool { return true }, time.Now().Add(time.Second), false)
			err := execute(context.Background(), func() error { applied++; return nil })
			if checks != 2 || (err == nil) != !revoke || (applied == 1) != !revoke {
				t.Fatal("late service proof reused stale desktop or replayed input", err, checks, applied)
			}
		})
	}
}

func TestNativeExecutionJoinsFailedDesktopAndLateServiceBeforeReturning(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started := time.Now()
		serviceFailure := errors.New("service query failed")
		execute := nativeInputExecutor(func(context.Context, func() error) error { return ErrAuthority }, func() error {
			time.Sleep(20 * time.Millisecond)
			return serviceFailure
		}, func() bool { return true }, time.Now().Add(time.Second), false)
		err := execute(context.Background(), func() error { t.Fatal("denied input applied"); return nil })
		if time.Since(started) != 20*time.Millisecond || !errors.Is(err, ErrAuthority) || !errors.Is(err, serviceFailure) {
			t.Fatal("authority handles could close before the late query returned", err, time.Since(started))
		}
	})
}

func TestNativeExecutionRepliesBeforeJoiningSlowProofAtDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		releaseProof := make(chan struct{})
		proofStarted, proofDone := make(chan struct{}), make(chan struct{})
		desktopChecks, applied := 0, 0
		execute := nativeInputExecutor(func(_ context.Context, work func() error) error {
			desktopChecks++
			return work()
		}, func() error {
			close(proofStarted)
			<-releaseProof
			close(proofDone)
			return nil
		}, func() bool { return true }, time.Now().Add(time.Second), false)
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
		defer cancel()
		started := time.Now()
		err := execute(ctx, func() error { applied++; return nil })
		elapsed := time.Since(started)
		if !errors.Is(err, context.DeadlineExceeded) || applied != 0 || desktopChecks != 1 || elapsed < 20*time.Millisecond || elapsed >= 40*time.Millisecond {
			t.Fatal("slow proof suppressed the not-applied reply budget", err, applied, desktopChecks, elapsed)
		}
		<-proofStarted
		close(releaseProof)
		<-proofDone
	})
}

func TestNativeProofTimeoutFencesLateSuccessAndRetainsAuthorityHandles(t *testing.T) {
	for _, mode := range []string{"deadline", "cancel", "no_deadline"} {
		synctest.Test(t, func(t *testing.T) {
			blocked, release, closed := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			authority := &Authority{checkInput: func() error {
				calls.Add(1)
				close(blocked)
				<-release
				return nil
			}, checkRecovery: func() error { return nil }, closeHandles: func() error { close(closed); return nil }}
			var timing workerTiming
			execute := nativeInputExecutor(func(_ context.Context, work func() error) error {
				return work()
			}, func() error { return authority.check(false) }, func() bool { return true }, time.Now().Add(time.Hour), false, &timing)
			ctx := context.Background()
			if mode == "deadline" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithTimeout(ctx, 40*time.Millisecond)
				defer cancel()
			} else if mode == "cancel" {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				go func() { <-blocked; cancel() }()
			}
			started := time.Now()
			err := beforeInputCertainty(execute)(ctx, func() error { t.Fatal("pending proof injected input"); return nil })
			certainty, ok := application.InjectionCertaintyOf(err)
			if !ok || certainty != application.InjectionNotApplied || !errors.Is(err, errNativeProofPending) || time.Since(started) > time.Second {
				t.Fatal("pending proof did not return bounded NOT_APPLIED", mode, err, time.Since(started))
			}
			<-blocked
			if authority.mu.TryLock() {
				authority.mu.Unlock()
				t.Fatal("pending query released its authority pin")
			}
			closeStarted := make(chan struct{})
			go func() { close(closeStarted); _ = authority.Close() }()
			<-closeStarted // RWMutex waits are not durable synctest blocking.
			select {
			case <-closed:
				t.Fatal("authority handles closed during query")
			default:
			}
			if err := execute(context.Background(), func() error { t.Fatal("fenced dispatcher applied input"); return nil }); err == nil || calls.Load() != 1 {
				t.Fatal("pending query spawned a replacement", err, calls.Load())
			}
			before := timing
			close(release)
			<-closed
			synctest.Wait()
			if timing != before {
				t.Fatal("late query mutated returned timing")
			}
			if err := execute(context.Background(), func() error { t.Fatal("late success revived input"); return nil }); err == nil || calls.Load() != 1 {
				t.Fatal("late proof revived dispatcher", err, calls.Load())
			}
		})
	}
}

func TestNativeExecutionContainsServicePanicWithoutAdmittingInput(t *testing.T) {
	execute := nativeInputExecutor(func(_ context.Context, work func() error) error { return work() },
		func() error { panic("private native data must not escape") }, func() bool { return true }, time.Now().Add(time.Second), false)
	err := execute(context.Background(), func() error { t.Fatal("panicking proof admitted input"); return nil })
	if !errors.Is(err, ErrAuthority) || !errors.Is(err, inputdesktop.ErrCallbackPanic) {
		t.Fatal("native proof panic lost the desktop cleanup fence", err)
	}
}

func TestNativeProofBudgetNeverRewritesAnAlreadyStartedInput(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		failure := unknownInput(errors.New("native operation did not confirm its result"))
		calls := 0
		execute := nativeInputExecutor(func(_ context.Context, work func() error) error {
			time.Sleep(time.Millisecond) // Let this command's fresh service check finish.
			return work()
		}, func() error { return nil }, func() bool { return true }, time.Now().Add(time.Hour), false)
		ctx, cancel := context.WithTimeout(context.Background(), IdleTimeout)
		defer cancel()
		err := beforeInputCertainty(execute)(ctx, func() error {
			calls++
			time.Sleep(3 * time.Second)
			return failure
		})
		certainty, known := application.InjectionCertaintyOf(err)
		if !errors.Is(err, failure) || !known || certainty != application.InjectionOutcomeUnknown || calls != 1 || errors.Is(err, errNativeProofPending) {
			t.Fatal("proof timeout rewrote or repeated an attempted input", err, calls)
		}
	})
}

func BenchmarkNativeExecutionFreshProofs(b *testing.B) {
	execute := nativeInputExecutor(func(_ context.Context, work func() error) error { return work() },
		func() error { return nil }, func() bool { return true }, time.Now().Add(time.Hour), false)
	ctx := context.Background()
	work := func() error { return nil }
	b.ReportAllocs()
	for b.Loop() {
		if err := execute(ctx, work); err != nil {
			b.Fatal(err)
		}
	}
}

func TestNativeExecutionRechecksExpiryAndCancellationAfterSlowProof(t *testing.T) {
	for _, mode := range []string{"expiry", "cancel", "release"} {
		synctest.Test(t, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			applied := 0
			execute := nativeInputExecutor(func(_ context.Context, work func() error) error { return work() }, func() error {
				time.Sleep(60 * time.Millisecond)
				if mode == "cancel" {
					cancel()
				}
				return nil
			}, func() bool { return true }, time.Now().Add(50*time.Millisecond), mode == "release")
			err := execute(ctx, func() error { applied++; return nil })
			if (err == nil) != (mode == "release") || (applied == 1) != (mode == "release") {
				t.Fatal("proof delay bypassed expiry/cancellation or blocked recovery", mode, err, applied)
			}
		})
	}
}
