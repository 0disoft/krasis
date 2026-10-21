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
	for _, op := range []byte{sourceAdmit, sourceHeartbeat} {
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
			owners[i].authority.checkInput = func() error { time.Sleep(600 * time.Millisecond); return nil }
		}
		started := time.Now()
		err := authorizeSourceRead(context.Background(), owners[0], owners[1], sourceAdmit)
		if !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != time.Second {
			t.Fatal("owners received separate budgets", err, time.Since(started))
		}
		// The second native owner still runs for 200ms after admission expires.
		if owners[1].Close() != ErrAuthority {
			t.Fatal("unfinished second owner reported cleanup")
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
