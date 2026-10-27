package productpanel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestInputMaintenanceHealthDeadlineRetainsInspectionGate(t *testing.T) {
	for _, phase := range []string{"preflight", "pending", "verification"} {
		t.Run(phase, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				entered, finish := make(chan struct{}), make(chan struct{})
				var probes, installs atomic.Int32
				b := &inputRefresh{inspect: func() string {
					probe := probes.Add(1)
					if phase != "verification" || probe == 2 {
						close(entered)
						<-finish
					}
					if phase == "verification" && probe == 2 {
						return "READY"
					}
					return "UPDATE_REQUIRED"
				}, run: func(context.Context) error { installs.Add(1); return nil }}
				if phase == "pending" {
					b.setState("PENDING")
				}
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				result := make(chan error, 1)
				elapsed := make(chan time.Duration, 1)
				go func() {
					started := time.Now()
					err := b.automatic(ctx)
					elapsed <- time.Since(started)
					result <- err
				}()
				<-entered
				time.Sleep(2 * time.Second)
				select {
				case err := <-result:
					if !errors.Is(err, context.DeadlineExceeded) {
						t.Fatal("health inspection lost caller deadline", err)
					}
				default:
					close(finish)
					<-result
					t.Fatal("native health inspection outlived the caller deadline")
				}
				if waited := <-elapsed; waited != time.Second {
					t.Fatal("caller did not return at its original deadline", waited)
				}
				reader := b.reader.Load()
				if reader == nil {
					t.Fatal("unfinished inspection lost its owner")
				}
				wantProbes, wantInstalls := int32(1), int32(0)
				if phase == "verification" {
					wantProbes, wantInstalls = 2, 1
				}
				for range 3 {
					b.status()
					b.recheck()
					if !errors.Is(b.explicit(context.Background()), ErrStartupBusy) {
						t.Fatal("unfinished inspection permitted concurrent installation")
					}
					if !errors.Is(b.restoreCheck(ctx), context.DeadlineExceeded) {
						t.Fatal("expired restoration observed a successful operation")
					}
				}
				if probes.Load() != wantProbes || installs.Load() != wantInstalls {
					t.Fatal("cancellation duplicated native work", probes.Load(), installs.Load())
				}
				close(finish)
				<-reader.done
				b.mu.Lock() // Join the inspection's actual retirement, not the caller.
				b.mu.Unlock()
				if installs.Load() != wantInstalls || probes.Load() != wantProbes {
					t.Fatal("late completion restarted installation", installs.Load(), probes.Load())
				}
				if phase == "verification" && (b.status() != "READY" || b.attemptResult() != "ok") {
					t.Fatal("late verified outcome was lost", b.status(), b.attemptResult())
				}
			})
		})
	}
}
