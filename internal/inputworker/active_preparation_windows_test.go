//go:build windows

package inputworker

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"
)

func TestActivePreparationSurvivesShortLeaseWithoutDuplicateReentry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		lease := Lease{ExpiresAt: time.Now().Add(time.Minute)}
		var slot serviceInputPreparation
		calls := 0
		child := &preparedConsole{preparedAt: time.Now()}
		build := func(context.Context) (*preparedConsole, error) { calls++; return child, nil }
		if !slot.start(ctx, build) {
			t.Fatal("acknowledged active lease could not start its spare")
		}
		synctest.Wait()
		// Active input continues while the unbound builder finishes. No timer
		// or later service request is needed to perform this work.
		time.Sleep(250 * time.Millisecond)
		if calls != 1 || slot.warming == nil {
			t.Fatal("spare was not built during active input")
		}
		if err := slot.reenter(ctx, lease, nil, build); err != nil || slot.next != child || slot.warming != nil || calls != 1 {
			t.Fatal("clean return discarded or duplicated its live spare", err)
		}
		if takeInputPreparation(&slot.next, Lease{}) != child || takeInputPreparation(&slot.next, Lease{}) != nil {
			t.Fatal("spare ownership did not transfer exactly once")
		}
		if err := child.close(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestActivePreparationExpiresWithoutRefreshThenReplacesAfterReturn(t *testing.T) {
	for _, cleanupFailure := range []bool{false, true} {
		t.Run(map[bool]string{false: "clean", true: "uncertain retirement"}[cleanupFailure], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx := context.Background()
				lease := Lease{ExpiresAt: time.Now().Add(time.Minute)}
				var slot serviceInputPreparation
				old := &preparedConsole{preparedAt: time.Now()}
				if cleanupFailure {
					old.worker = failedConsoleCloseForTest()
				}
				old.warmTimer = time.AfterFunc(preparedConsoleWindow, func() { _ = old.expireUnbound() })
				calls := 0
				slot.start(ctx, func(context.Context) (*preparedConsole, error) { calls++; return old, nil })
				synctest.Wait()
				time.Sleep(preparedConsoleWindow + time.Millisecond)
				if !old.retired() || calls != 1 {
					t.Fatal("active lease extended expiry or refreshed its spare")
				}
				fresh := &preparedConsole{preparedAt: time.Now()}
				if err := slot.reenter(ctx, lease, nil, func(context.Context) (*preparedConsole, error) { calls++; return fresh, nil }); err != nil {
					t.Fatal(err)
				}
				channel := &Channel{}
				err := slot.admit(channel)
				if cleanupFailure {
					if !errors.Is(err, ErrRecovery) || !channel.closing || slot.next != nil || calls != 1 {
						t.Fatal("uncertain spare retirement admitted a replacement", err)
					}
				} else if err != nil || channel.closing || slot.next != fresh || calls != 2 {
					t.Fatal("long active lease left an expired spare blocking reentry", err)
				}
				if err := slot.close(); err != nil {
					t.Fatal(err)
				}
			})
		})
	}
}

func TestActivePreparationFailureCannotDisappearAtCleanReturn(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var slot serviceInputPreparation
		cause := errors.New("unconfirmed spare native exit")
		slot.start(context.Background(), func(context.Context) (*preparedConsole, error) {
			return nil, errors.Join(ErrRecovery, cause)
		})
		synctest.Wait()
		err := slot.reenter(context.Background(), Lease{ExpiresAt: time.Now().Add(time.Minute)}, nil, func(context.Context) (*preparedConsole, error) {
			t.Error("new work replaced an unconfirmed retirement")
			return nil, nil
		})
		if !errors.Is(err, ErrRecovery) || !errors.Is(err, cause) || slot.next != nil || slot.warming != nil {
			t.Fatal("clean active return hid a completed preparation failure", err)
		}
	})
}
