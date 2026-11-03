//go:build windows

package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func reentrySpare(worker ...*ConsoleWorker) *preparedConsole {
	p := &preparedConsole{preparedAt: time.Now()}
	if len(worker) == 1 {
		p.worker = worker[0]
	}
	p.warmTimer = time.AfterFunc(preparedConsoleWindow, func() { _ = p.expireUnbound() })
	return p
}

func TestReentryOpportunityRetainsOriginalSpareAcrossFiveSecondPause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		old := reentrySpare()
		slot := serviceInputPreparation{next: old}
		defer slot.close()
		builds := 0
		if err := slot.reenter(ctx, Lease{ExpiresAt: time.Now().Add(time.Minute)}, nil, func(context.Context) (*preparedConsole, error) {
			builds++
			time.Sleep(130 * time.Millisecond) // Native loading model, no real input.
			return reentrySpare(), nil
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Second)
		synctest.Wait()
		started := time.Now()
		child, err := slot.take(&Channel{}, Lease{})
		if err != nil || child != old || child.retired() || builds != 0 || time.Since(started) != 0 {
			t.Fatal("five-second entry rebuilt or lost its existing unbound spare", err, builds, time.Since(started))
		}
		_ = child.close()
		time.Sleep(preparedConsoleWindow)
		synctest.Wait()
		if builds != 0 || slot.reentry != nil {
			t.Fatal("one return started periodic idle construction", builds)
		}
	})
}

func TestReentrySpareRetainsNativeReservationAcrossSixMinuteIdle(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		child := reentrySpare()
		slot := serviceInputPreparation{next: child}
		defer slot.close()
		time.Sleep(6 * time.Minute) // g521's slow entry followed a multi-minute gap.
		if child.retired() {
			t.Fatal("idle discarded a still-bounded native reservation")
		}
		got, err := slot.take(&Channel{}, Lease{})
		if err != nil || got != child || slot.next != nil {
			t.Fatal("idle entry lost the existing exact owner", err)
		}
		if err := got.close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestReentryOpportunityNeverRetiresTheInitialSpareEarly(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var slot serviceInputPreparation
		defer slot.close()
		var builds atomic.Int32
		var first atomic.Pointer[preparedConsole]
		if err := slot.reenter(context.Background(), Lease{ExpiresAt: time.Now().Add(preparedConsoleWindow + time.Minute)}, nil, func(context.Context) (*preparedConsole, error) {
			n := builds.Add(1)
			time.Sleep(150 * time.Millisecond)
			p := reentrySpare()
			if n == 1 {
				first.Store(p)
			}
			return p, nil
		}); err != nil {
			t.Fatal(err)
		}
		time.Sleep(preparedConsoleWindow)
		synctest.Wait()
		if p := first.Load(); p == nil || p.retired() || builds.Load() != 1 {
			t.Fatal("late-completed initial spare retired before its original expiry")
		}
		time.Sleep(time.Second)
		synctest.Wait()
		if p := first.Load(); builds.Load() != 2 || p == nil || !p.retired() {
			t.Fatal("one opportunity missed the actual initial expiry", builds.Load())
		}
	})
}

func TestReentryOpportunityCancelledByEntryFreshDemandShutdownOrExpiredLease(t *testing.T) {
	for _, stop := range []string{"entry", "hint", "shutdown", "expiry", "cancel"} {
		t.Run(stop, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				old := reentrySpare()
				slot := serviceInputPreparation{next: old}
				defer slot.close()
				builds := 0
				build := func(context.Context) (*preparedConsole, error) { builds++; return reentrySpare(), nil }
				expires := time.Now().Add(time.Minute)
				if stop == "expiry" {
					expires = time.Now().Add(time.Second)
				}
				if err := slot.reenter(ctx, Lease{ExpiresAt: expires}, nil, build); err != nil {
					t.Fatal(err)
				}
				switch stop {
				case "entry":
					child, err := slot.take(&Channel{}, Lease{})
					if err != nil || child != old {
						t.Fatal("early entry lost warm spare", err)
					}
					_ = child.close()
				case "hint":
					// An old spare can expire shortly after return. A real demand
					// replaces it before the delayed opportunity; only that demand runs.
					_ = old.expireUnbound()
					time.Sleep(2100 * time.Millisecond)
					if !slot.hint(ctx, time.Time{}, time.Now(), build) {
						t.Fatal("fresh demand did not replace expired spare")
					}
				case "shutdown":
					_ = slot.close()
				case "cancel":
					cancel()
				}
				time.Sleep(preparedConsoleWindow + time.Second)
				synctest.Wait()
				want := 0
				if stop == "hint" {
					want = 1
				}
				if builds != want || slot.reentry != nil {
					t.Fatal("obsolete delayed opportunity created a spare", stop, builds)
				}
			})
		})
	}
}

func TestReentryOpportunityKeepsUncertainRetirementTerminal(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		old := reentrySpare(failedConsoleCloseForTest())
		slot := serviceInputPreparation{next: old}
		defer slot.close()
		builds := 0
		if err := slot.reenter(context.Background(), Lease{ExpiresAt: time.Now().Add(preparedConsoleWindow + time.Minute)}, nil, func(context.Context) (*preparedConsole, error) { builds++; return reentrySpare(), nil }); err != nil {
			t.Fatal(err)
		}
		time.Sleep(preparedConsoleWindow + time.Second)
		synctest.Wait()
		channel := &Channel{}
		if child, err := slot.take(channel, Lease{}); child != nil || !errors.Is(err, ErrRecovery) || !channel.closing || builds != 0 {
			t.Fatal("delayed replacement bypassed exact retirement", err, builds)
		}
	})
}
