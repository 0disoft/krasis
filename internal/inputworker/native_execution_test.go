package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
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
