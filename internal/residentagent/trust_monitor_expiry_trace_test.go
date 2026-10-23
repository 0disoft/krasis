package residentagent

import (
	"context"
	"slices"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func TestTrustExpiryRecordsCompletedReadAfterObserverStall(t *testing.T) {
	for _, lateRead := range []bool{false, true} {
		t.Run(map[bool]string{false: "stale-success", true: "late-completion"}[lateRead], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ticks := make(chan time.Time, 1)
				ticks <- time.Now()
				if lateRead {
					go func() {
						time.Sleep(time.Second)
						ticks <- time.Now()
					}()
				}
				var calls int
				var codes []string
				var snapshots []truststore.ReadTraceSnapshot
				monitorProductSessionObserved(ctx, ticks, nil, func(trace *truststore.ReadTrace) error {
					calls++
					trace.EnterRetryWait()
					if calls == 1 {
						time.Sleep(60 * time.Millisecond)
					} else {
						time.Sleep(5 * time.Second)
					}
					return nil
				}, cancel, func(code string) { codes = append(codes, code) }, func(snapshot truststore.ReadTraceSnapshot) {
					snapshots = append(snapshots, snapshot)
					if len(snapshots) == 1 {
						time.Sleep(7 * time.Second)
					}
				})
				synctest.Wait()
				want := "trust_expiry_result_ok_before_deadline"
				if lateRead {
					want = "trust_expiry_result_error_late"
				}
				if ctx.Err() != context.Canceled || len(codes) != 3 || codes[0] != want ||
					codes[1] != "trust_expiry_late_ge1s" || codes[2] != "session_trust_freshness_expired" {
					t.Fatal("completed read expiry evidence lost or authority revived", codes, ctx.Err())
				}
				if len(snapshots) != 2 || !slices.ContainsFunc(snapshots, func(snapshot truststore.ReadTraceSnapshot) bool {
					return snapshot.Elapsed >= time.Second
				}) {
					t.Fatal("terminal completed trace was not retained", snapshots)
				}
			})
		})
	}
}

func TestTrustExpiryCapturesPhaseAfterSoftObservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ticks := make(chan time.Time, 1)
		ticks <- time.Now()
		release := make(chan struct{})
		var snapshots []truststore.ReadTraceSnapshot
		var codes []string
		monitorProductSessionObserved(ctx, ticks, nil, func(trace *truststore.ReadTrace) error {
			time.Sleep(2 * time.Second)
			trace.EnterRetryWait()
			<-release
			return nil
		}, cancel, func(code string) { codes = append(codes, code) }, func(snapshot truststore.ReadTraceSnapshot) {
			snapshots = append(snapshots, snapshot)
		})
		close(release)
		synctest.Wait()
		if ctx.Err() != context.Canceled || len(codes) != 4 || codes[1] != "trust_expiry_reading" || codes[2] != "trust_expiry_late_lt1ms" || codes[3] != "session_trust_freshness_expired" {
			t.Fatal("expiry or late-success policy changed", codes, ctx.Err())
		}
		if len(snapshots) != 2 || snapshots[0].Phase != "" || snapshots[1].Phase != "retry_wait" || snapshots[1].Elapsed != trustValidationMaxAge {
			t.Fatal("missing final blocked phase", snapshots)
		}
	})
}
