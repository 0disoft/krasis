package residentagent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func TestBlockedTrustReadCannotDelayGuardStopOrResumeSession(t *testing.T) {
	for _, guardExit := range []bool{false, true} {
		t.Run(map[bool]string{false: "read-timeout", true: "guard-exit"}[guardExit], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ticks := make(chan time.Time, 1)
				ticks <- time.Now()
				guard := make(chan struct{})
				release := make(chan struct{})
				entered := make(chan struct{})
				finished := make(chan struct{})
				var codes []string
				go func() {
					defer close(finished)
					monitorProductSession(ctx, ticks, guard, func() error { close(entered); <-release; return nil }, cancel, func(code string) {
						if ctx.Err() != nil {
							t.Error("recorded after cancellation")
						}
						codes = append(codes, code)
					})
				}()
				<-entered
				started := time.Now()
				if guardExit {
					close(guard)
				}
				<-finished
				want := "session_trust_read_timeout"
				elapsed := trustReadTimeout
				if guardExit {
					want = "session_guard_exited"
					elapsed = 0
				}
				if len(codes) != 1 || codes[0] != want || time.Since(started) != elapsed || ctx.Err() != context.Canceled {
					t.Errorf("stop: %v, %s, %v", codes, time.Since(started), ctx.Err())
				}
				if err := boundedMonitoredTrust(context.Background(), nil, func() error { t.Error("started overlapping blocked read"); return nil }); !errors.Is(err, errTrustReadPending) {
					t.Errorf("unbounded workers: %v", err)
				}
				close(release)
				synctest.Wait()
				if ctx.Err() != context.Canceled || len(codes) != 1 {
					t.Fatal("late trust success changed stopped session")
				}
			})
		})
	}
}

func TestTrustMonitorSeparatesRevocationAndStoreFailures(t *testing.T) {
	for _, fixture := range []struct {
		cause error
		code  string
	}{
		{truststore.ErrPeerRevoked, "session_trust_revoked"}, {truststore.ErrPeerNotFound, "session_trust_peer_missing"},
		{truststore.ErrStoreNotFound, "session_trust_store_missing"}, {errors.New("private filesystem path"), "session_trust_check_failed"},
	} {
		t.Run(fixture.code, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ticks := make(chan time.Time, 1)
			ticks <- time.Now()
			var code string
			monitorProductSession(ctx, ticks, nil, func() error { return fixture.cause }, cancel, func(value string) { code = value })
			if code != fixture.code || ctx.Err() != context.Canceled {
				t.Fatalf("monitor: %s, %v", code, ctx.Err())
			}
		})
	}
}

func TestCertificateExpiryDuringTrustReadKeepsDeadlineReason(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		release := make(chan struct{})
		defer close(release)
		ticks := make(chan time.Time, 1)
		ticks <- time.Now()
		var code string
		monitorProductSession(ctx, ticks, nil, func() error { <-release; return nil }, cancel, func(value string) { code = value })
		if code != "session_certificate_deadline" {
			t.Fatal(code)
		}
	})
}

func TestTrustReadTimeoutReportsCurrentPhaseAndKeepsSessionStopped(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ticks := make(chan time.Time, 1)
		ticks <- time.Now()
		entered := make(chan struct{})
		release := make(chan struct{})
		finished := make(chan struct{})
		var codes []string
		go func() {
			defer close(finished)
			monitorProductSessionObserved(ctx, ticks, nil, func(trace *truststore.ReadTrace) error {
				trace.EnterRetryWait()
				close(entered)
				<-release
				return nil
			}, cancel, func(code string) { codes = append(codes, code) }, func(snapshot truststore.ReadTraceSnapshot) {
				recordTrustTrace(snapshot, func(code string) { codes = append(codes, code) })
			})
		}()
		<-entered
		<-finished
		if ctx.Err() != context.Canceled || len(codes) != 3 || codes[0] != "trust_timing_retry_wait_ge1s" || codes[1] != "trust_active_retry_wait" || codes[2] != "session_trust_read_timeout" {
			t.Fatalf("timeout evidence and fail-closed stop: %v, %v", codes, ctx.Err())
		}
		close(release)
		synctest.Wait()
		if len(codes) != 3 {
			t.Fatalf("late success altered stopped session: %v", codes)
		}
	})
}
