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
				want := []string{"session_trust_read_deferred", "session_trust_freshness_expired"}
				elapsed := trustValidationMaxAge
				if guardExit {
					want = []string{"session_guard_exited"}
					elapsed = 0
				}
				if len(codes) != len(want) || time.Since(started) != elapsed || ctx.Err() != context.Canceled {
					t.Fatalf("stop: %v, %s, %v", codes, time.Since(started), ctx.Err())
				}
				for i := range want {
					if codes[i] != want[i] {
						t.Fatalf("stop codes: %v", codes)
					}
				}
				if err := boundedMonitoredTrust(context.Background(), nil, func() error { t.Error("started overlapping blocked read"); return nil }); !errors.Is(err, errTrustReadPending) {
					t.Errorf("unbounded workers: %v", err)
				}
				close(release)
				synctest.Wait()
				if ctx.Err() != context.Canceled || len(codes) != len(want) {
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
		if ctx.Err() != context.Canceled || len(codes) != 4 || codes[0] != "trust_timing_retry_wait_ge1s" || codes[1] != "trust_active_retry_wait" || codes[2] != "session_trust_read_deferred" || codes[3] != "session_trust_freshness_expired" {
			t.Fatalf("timeout evidence and fail-closed stop: %v, %v", codes, ctx.Err())
		}
		close(release)
		synctest.Wait()
		if len(codes) != 4 {
			t.Fatalf("late success altered stopped session: %v", codes)
		}
	})
}

func TestTransientTrustReadFailureRecoversWithoutDisconnect(t *testing.T) {
	for _, tc := range []struct {
		name  string
		cause error
		calls int
	}{
		{"timeout", errTrustReadTimeout, 1},
		{"lock contention", truststore.ErrStoreBusy, 5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ticks := make(chan time.Time, 2)
				events := make(chan string, 3)
				finished := make(chan struct{})
				calls := 0
				go func() {
					defer close(finished)
					monitorProductSession(ctx, ticks, nil, func() error {
						calls++
						if calls <= tc.calls {
							return tc.cause
						}
						return nil
					}, cancel, func(code string) { events <- code })
				}()
				ticks <- time.Now()
				if code := <-events; code != "session_trust_read_deferred" || ctx.Err() != nil {
					t.Fatalf("first inconclusive read ended session: %s, %v", code, ctx.Err())
				}
				ticks <- time.Now()
				if code := <-events; code != "session_trust_read_recovered" || ctx.Err() != nil {
					t.Fatalf("retry did not recover: %s, %v", code, ctx.Err())
				}
				cancel()
				<-finished
				if calls != tc.calls+1 || len(events) != 0 {
					t.Fatalf("unexpected follow-up: calls=%d events=%d", calls, len(events))
				}
			})
		})
	}
}

func TestPersistentTrustLockContentionStopsAtFreshnessDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ticks := make(chan time.Time, 1)
		events := make(chan string, 2)
		finished := make(chan struct{})
		go func() {
			defer close(finished)
			monitorProductSession(ctx, ticks, nil, func() error { return truststore.ErrStoreBusy }, cancel, func(code string) { events <- code })
		}()
		ticks <- time.Now()
		if code := <-events; code != "session_trust_read_deferred" || ctx.Err() != nil {
			t.Fatalf("contention stopped before freshness deadline: %s, %v", code, ctx.Err())
		}
		<-finished
		if code := <-events; code != "session_trust_freshness_expired" || ctx.Err() != context.Canceled || len(events) != 0 {
			t.Fatalf("contention did not stop safely: %s, %v", code, ctx.Err())
		}
	})
}

func TestRevocationStillStopsDuringTrustReadGrace(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ticks := make(chan time.Time, 2)
		events := make(chan string, 2)
		finished := make(chan struct{})
		calls := 0
		go func() {
			defer close(finished)
			monitorProductSession(ctx, ticks, nil, func() error {
				calls++
				if calls == 1 {
					return errTrustReadTimeout
				}
				return truststore.ErrPeerRevoked
			}, cancel, func(code string) { events <- code })
		}()
		ticks <- time.Now()
		if code := <-events; code != "session_trust_read_deferred" {
			t.Fatal(code)
		}
		ticks <- time.Now()
		if code := <-events; code != "session_trust_revoked" {
			t.Fatal(code)
		}
		<-finished
		if ctx.Err() != context.Canceled || calls != 2 {
			t.Fatalf("revocation did not stop: %v, %d", ctx.Err(), calls)
		}
	})
}

func TestTrustTraceReportsNestedTimeoutStep(t *testing.T) {
	snapshot := truststore.ReadTraceSnapshot{
		Phase: "store_read_acl",
		Durations: map[string]time.Duration{
			"store_read":     time.Second,
			"store_read_acl": time.Second,
		},
	}
	var codes []string
	recordTrustTrace(snapshot, func(code string) { codes = append(codes, code) })
	if len(codes) != 3 || codes[0] != "trust_timing_store_read_ge1s" ||
		codes[1] != "trust_timing_store_read_acl_ge1s" || codes[2] != "trust_active_store_read_acl" {
		t.Fatalf("nested timeout codes: %v", codes)
	}
}

func TestTrustTraceReportsActiveParentACLCall(t *testing.T) {
	snapshot := truststore.ReadTraceSnapshot{
		Phase: "prelock_parent_acl_query",
		Durations: map[string]time.Duration{
			"prelock":            time.Second,
			"prelock_parent_acl": time.Second,
		},
	}
	var codes []string
	recordTrustTrace(snapshot, func(code string) { codes = append(codes, code) })
	if len(codes) != 3 || codes[0] != "trust_timing_prelock_ge1s" ||
		codes[1] != "trust_timing_prelock_parent_acl_ge1s" || codes[2] != "trust_active_prelock_parent_acl_query" {
		t.Fatalf("active parent ACL call: %v", codes)
	}
}

func TestTrustTraceOmitsFastCompletedSubsteps(t *testing.T) {
	snapshot := truststore.ReadTraceSnapshot{
		Durations: map[string]time.Duration{
			"prelock":                100 * time.Millisecond,
			"prelock_parent_acl":     time.Millisecond,
			"prelock_directory_open": 99 * time.Millisecond,
		},
	}
	var codes []string
	recordTrustTrace(snapshot, func(code string) { codes = append(codes, code) })
	if len(codes) != 2 || codes[0] != "trust_timing_prelock_lt1s" ||
		codes[1] != "trust_timing_prelock_directory_open_lt100ms" {
		t.Fatalf("slow substep codes: %v", codes)
	}
}
