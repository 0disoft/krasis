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

func TestSourceReaderBoundsBothNativeOwnersAndRetainsHandles(t *testing.T) {
	for _, op := range []byte{sourceStart, sourceHeartbeat} {
		for _, stalled := range []int{0, 1} {
			synctest.Test(t, func(t *testing.T) {
				owners := make([]*Channel, 2)
				streams := make([]*queryOwnedConn, 2)
				var closes [2]atomic.Int32
				for i := range owners {
					server, pipe := net.Pipe()
					defer server.Close()
					streams[i] = &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
					owners[i] = clientChannel(streams[i], authorityService)
					owners[i].authority.closeHandles = func() error { closes[i].Add(1); return nil }
				}
				release := make(chan struct{})
				native := func() error {
					<-release
					if closes[stalled].Load() != 0 || streams[stalled].closes.Load() != 0 {
						t.Error("native read query lost its proof or pipe")
					}
					return nil
				}
				if op == sourceHeartbeat {
					owners[stalled].authority.checkPresent = native
				} else {
					owners[stalled].authority.checkInput = native
				}
				timer := time.AfterFunc(2*time.Second, func() { close(release) })
				ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
				defer cancel()
				started := time.Now()
				err := authorizeSourceRead(ctx, owners[0], owners[1], op)
				if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond {
					t.Errorf("read query defeated deadline: op=%d owner=%d elapsed=%s err=%v", op, stalled, time.Since(started), err)
				}
				if closes[stalled].Load() != 0 || owners[stalled].Close() != ErrAuthority {
					t.Error("pending native owner reported cleanup")
				}
				if timer.Stop() {
					close(release)
				}
				synctest.Wait()
				for i := range owners {
					if owners[i].Close() != nil || closes[i].Load() != 1 || streams[i].closes.Load() != 1 {
						t.Error("native owner failed exactly-once retirement", i)
					}
				}
			})
		}
	}
}

func TestSourceReaderUsesOneBudgetAcrossBothOwners(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owners := make([]*Channel, 2)
		for i := range owners {
			server, pipe := net.Pipe()
			defer server.Close()
			owners[i] = clientChannel(pipe, authorityService)
			owners[i].authority.checkInput = func() error { time.Sleep(1200 * time.Millisecond); return nil }
		}
		started := time.Now()
		err := authorizeSourceExecution(context.Background(), owners[0], owners[1])
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != time.Second {
			t.Fatal("owners received separate budgets", err, time.Since(started))
		}
		// Both native owners still run for 200ms after the shared budget expires.
		for _, owner := range owners {
			if owner.Close() != ErrAuthority {
				t.Fatal("unfinished owner reported cleanup")
			}
		}
		time.Sleep(200 * time.Millisecond)
		synctest.Wait()
		for _, owner := range owners {
			if owner.Close() != nil {
				t.Fatal("completed read query leaked resources")
			}
		}
	})
}

func TestSourceExecutionOverlapsIndependentFreshProofs(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owners := make([]*Channel, 2)
		defer func() {
			// A failing assertion must also join an expired native query.
			time.Sleep(time.Second)
			synctest.Wait()
			for _, owner := range owners {
				if owner.Close() != nil {
					t.Error("completed proof retained native resources")
				}
			}
		}()
		var calls [2]atomic.Int32
		for i := range owners {
			server, pipe := net.Pipe()
			defer server.Close()
			owners[i] = clientChannel(pipe, authorityService)
			owners[i].authority.checkInput = func() error {
				calls[i].Add(1)
				time.Sleep(600 * time.Millisecond)
				return nil
			}
		}
		for attempt := int32(1); attempt <= 2; attempt++ {
			started := time.Now()
			if err := authorizeSourceExecution(context.Background(), owners[0], owners[1]); err != nil || time.Since(started) != 600*time.Millisecond {
				t.Fatal("independent proofs serialized or inherited separate budgets", err, time.Since(started))
			}
			if calls[0].Load() != attempt || calls[1].Load() != attempt {
				t.Fatal("execution reused a previous authority proof")
			}
		}
	})
}

func TestSourceExecutionRejectsEitherOwnerAndJoinsBothProofs(t *testing.T) {
	for _, rejected := range []int{0, 1} {
		synctest.Test(t, func(t *testing.T) {
			owners := make([]*Channel, 2)
			var completed [2]atomic.Bool
			for i := range owners {
				server, pipe := net.Pipe()
				defer server.Close()
				owners[i] = clientChannel(pipe, authorityService)
				owners[i].authority.checkInput = func() error {
					time.Sleep(time.Duration(i+1) * 10 * time.Millisecond)
					completed[i].Store(true)
					if i == rejected {
						return ErrAuthority
					}
					return nil
				}
			}
			err := authorizeSourceExecution(context.Background(), owners[0], owners[1])
			if !errors.Is(err, ErrAuthority) || !completed[0].Load() || !completed[1].Load() {
				t.Fatal("denied or unfinished proof granted capture", rejected, err)
			}
			for _, owner := range owners {
				if owner.Close() != nil {
					t.Fatal("proof resources leaked")
				}
			}
		})
	}
}
