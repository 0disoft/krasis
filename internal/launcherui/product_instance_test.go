package launcherui

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/agentinstance"
)

type windowTestLease struct{ closed int }

func (l *windowTestLease) Close() error { l.closed++; return nil }

func TestClaimProductWindowOwnsOrReusesBeforeStartup(t *testing.T) {
	for _, existing := range []bool{false, true} {
		lease := &windowTestLease{}
		got, err := claimProductWindow(context.Background(), func() (io.Closer, error) { return lease, nil }, func() bool { return existing })
		if err != nil {
			t.Fatal(err)
		}
		if existing {
			if got != nil || lease.closed != 1 {
				t.Fatal("legacy window did not release new ownership")
			}
		} else {
			if got != lease || lease.closed != 0 {
				t.Fatal("first instance lost ownership")
			}
			got.Close()
		}
	}
}

func TestClaimProductWindowWaitsForOwnerAndRecoversAfterExit(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, exits := range []bool{false, true} {
			attempts := 0
			lease := &windowTestLease{}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			got, err := claimProductWindow(ctx, func() (io.Closer, error) {
				attempts++
				if exits && attempts == 3 {
					return lease, nil
				}
				return nil, agentinstance.ErrAlreadyRunning
			}, func() bool { return !exits && attempts == 3 })
			cancel()
			if err != nil || attempts != 3 {
				t.Fatalf("attempts=%d err=%v", attempts, err)
			}
			if exits && got != lease {
				t.Fatal("dead owner prevented recovery")
			}
			if !exits && got != nil {
				t.Fatal("duplicate obtained ownership")
			}
			if got != nil {
				got.Close()
			}
		}
	})
}

func TestClaimProductWindowFailsClosedOnMissingWindowOrDeniedLock(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		for _, cause := range []error{agentinstance.ErrAlreadyRunning, errors.New("permission denied")} {
			ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
			got, err := claimProductWindow(ctx, func() (io.Closer, error) { return nil, cause }, func() bool { return false })
			cancel()
			if got != nil {
				t.Fatal("created duplicate after failed lock")
			}
			want := cause
			if cause == agentinstance.ErrAlreadyRunning {
				want = context.DeadlineExceeded
			}
			if !errors.Is(err, want) {
				t.Fatalf("err=%v want=%v", err, want)
			}
		}
	})
}

func TestControlLogWarningDistinguishesCausesWithoutRawErrors(t *testing.T) {
	healthy := controlLogWarning(true, false, nil)
	busy := controlLogWarning(false, false, agentinstance.ErrAlreadyRunning)
	unavailable := controlLogWarning(false, false, errors.New("private-path-marker"))
	dropped := controlLogWarning(true, true, nil)
	if healthy != "" || busy == "" || unavailable == "" || dropped == "" || busy == unavailable || unavailable == dropped || busy == dropped {
		t.Fatal("log health states collapsed")
	}
	if strings.Contains(unavailable, "private-path-marker") {
		t.Fatal("raw log error exposed")
	}
}
