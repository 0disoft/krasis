package residentagent

import (
	"context"
	"errors"
	"runtime"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
)

func TestStalledProtectedInspectionCannotHoldPressGateOrReplayLateClear(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, release := make(chan struct{}), make(chan struct{})
		var checks atomic.Int32
		inspect := boundedProtectedInputInspection(func() bool {
			if checks.Add(1) == 1 {
				close(entered)
				<-release // Deliberately ignore the caller's cancellation.
			}
			return false
		})
		var events []string
		input := &guardedProductInput{InputBoundaryPort: safetyInput{events: &events}, guard: safetyGuard{&events}, protected: inspect}
		token := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		done := make(chan error, 1)
		go func() { done <- input.Press(ctx, token) }()
		<-entered
		err := <-done
		certainty, known := application.InjectionCertaintyOf(err)
		if !errors.Is(err, context.DeadlineExceeded) || !known || certainty != application.InjectionNotApplied || len(events) != 0 {
			t.Fatal("expired inspection admitted guard/native input", err, events)
		}
		if err := input.Release(context.Background(), token); err != nil || len(events) != 2 {
			t.Fatal("read-only inspection blocked independent release", err, events)
		}
		close(release)
		synctest.Wait()
		if len(events) != 2 {
			t.Fatal("late clear result replayed an expired press", events)
		}
		if err := input.Press(context.Background(), token); err != nil || checks.Load() != 2 || len(events) != 4 {
			t.Fatal("fresh input reused an earlier inspection", err, checks.Load(), events)
		}
		synctest.Wait()
	})
}

func TestProtectedInspectionCapacityIncludesExpiredNativeQueries(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		entered := make(chan struct{}, cap(protectedInspectionSlots))
		done := make(chan error, cap(protectedInspectionSlots))
		var checks atomic.Int32
		for range cap(protectedInspectionSlots) {
			inspect := boundedProtectedInputInspection(func() bool {
				checks.Add(1)
				entered <- struct{}{}
				<-release
				return false
			})
			go func() {
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				_, err := inspect(ctx)
				done <- err
			}()
		}
		for range cap(protectedInspectionSlots) {
			<-entered
		}
		for range cap(protectedInspectionSlots) {
			if err := <-done; !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		}
		replacement := boundedProtectedInputInspection(func() bool { checks.Add(1); return false })
		if _, err := replacement(context.Background()); !errors.Is(err, errProtectedInspectionUnavailable) || int(checks.Load()) != cap(protectedInspectionSlots) {
			t.Fatal("expired queries lost their global resource slots", err, checks.Load())
		}
		close(release)
		synctest.Wait()
		if active, err := replacement(context.Background()); err != nil || active {
			t.Fatal("confirmed query retirement did not restore capacity", active, err)
		}
		synctest.Wait()
		if len(protectedInspectionSlots) != 0 {
			t.Fatal("completed inspections retained capacity")
		}
	})
}

func TestProtectedInspectionPanicAndGoexitCannotProduceClearResult(t *testing.T) {
	for _, inspect := range []func() bool{
		func() bool { panic("private native detail") },
		func() bool { runtime.Goexit(); return false },
	} {
		synctest.Test(t, func(t *testing.T) {
			active, err := boundedProtectedInputInspection(inspect)(context.Background())
			if active || err != errProtectedInspectionUnavailable {
				t.Fatal("abnormal native query admitted input or leaked its payload", active, err)
			}
			synctest.Wait()
			if len(protectedInspectionSlots) != 0 {
				t.Fatal("abnormal query retained capacity")
			}
		})
	}
}

func TestProtectedMonitorCancellationDoesNotJoinStalledInspection(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, release := make(chan struct{}), make(chan struct{})
		inspect := boundedProtectedInputInspection(func() bool { close(entered); <-release; return false })
		ctx, cancel := context.WithCancel(context.Background())
		ticks := make(chan time.Time, 1)
		ticks <- time.Now()
		done := make(chan error, 1)
		go func() {
			done <- monitorProtectedInput(ctx, ticks, inspect, func() { t.Error("unknown inspection became protected-screen evidence") })
		}()
		<-entered
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal("monitor cleanup waited for native query", err)
		}
		close(release)
		synctest.Wait()
	})
}

func TestProtectedInspectionRecoveryWaitsForExactNativeQueryRetirement(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		inspect := boundedProtectedInputInspection(func() bool { <-release; return false })
		finished := errors.New("fresh runner completed")
		attempts, waits := 0, 0
		operations := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error {
			attempts++
			if attempts == 1 {
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				_, err := inspect(ctx)
				return err
			}
			if len(protectedInspectionSlots) != 0 {
				t.Fatal("new runner overlapped an expired native inspection")
			}
			return finished
		})
		operations.sessionDone = make(chan struct{})
		err := operations.runWithProtectedRecovery(context.Background(), "peer", &sessionObserver{operations: operations, done: operations.sessionDone}, func() bool {
			t.Fatal("late inspection became protected-screen evidence")
			return false
		}, func(context.Context, time.Duration) error {
			waits++
			if waits == 2 {
				close(release)
				synctest.Wait()
			}
			if attempts != 1 {
				t.Fatal("inspection retry skipped its bounded wait")
			}
			return nil
		})
		if !errors.Is(err, finished) || waits != 2 || attempts != 2 {
			t.Fatal("connection intent lost or native owner replaced early", err, waits, attempts)
		}
	})
}

func TestProtectedInspectionRetryCannotHideCleanupOrUnknownInjection(t *testing.T) {
	expiry := &protectedInspectionExpiry{cause: context.DeadlineExceeded}
	if !protectedInspectionExpiredOnly(&application.InjectionError{Certainty: application.InjectionNotApplied, Cause: expiry}) {
		t.Fatal("proven unsubmitted inspection timeout rejected")
	}
	for _, err := range []error{
		context.DeadlineExceeded, errProtectedInspectionUnavailable,
		errors.Join(expiry, context.DeadlineExceeded),
		errors.Join(expiry, errors.New("release failed")),
		&application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: expiry},
		&application.InjectionError{Certainty: application.InjectionPartiallyApplied, Cause: expiry},
	} {
		if protectedInspectionExpiredOnly(err) {
			t.Fatal("unverified cleanup or input enabled automatic retry", err)
		}
	}
	operations := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error { return expiry })
	operations.sessionDone = make(chan struct{})
	operations.manualStop = true
	if err := operations.runWithProtectedRecovery(context.Background(), "peer", &sessionObserver{operations: operations, done: operations.sessionDone}, func() bool { return false }, func(context.Context, time.Duration) error { t.Fatal("manual stop retried"); return nil }); err != expiry {
		t.Fatal(err)
	}
}
