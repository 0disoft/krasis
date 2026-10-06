package residentagent

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func TestSlowTrustReadCompletesInsideFreshnessWithoutAnotherTick(t *testing.T) {
	for _, duration := range []time.Duration{1500 * time.Millisecond, 4 * time.Second} {
		t.Run(duration.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ticks := make(chan time.Time, 1)
				events := make(chan string, 4)
				done := make(chan struct{})
				calls := 0
				go func() {
					defer close(done)
					monitorProductSession(ctx, ticks, nil, func() error {
						calls++
						time.Sleep(duration)
						return nil
					}, cancel, func(code string) { events <- code })
				}()
				started := time.Now()
				ticks <- started
				if code := <-events; code != "session_trust_read_deferred" || ctx.Err() != nil {
					t.Fatal(code, ctx.Err())
				}
				// A tick while the original read is running cannot start another.
				ticks <- time.Now()
				if code := <-events; code != "session_trust_read_recovered" || ctx.Err() != nil || calls != 1 || time.Since(started) != duration {
					t.Fatal(code, ctx.Err(), calls, time.Since(started))
				}
				// Without further ticks, expiry is measured from validation
				// completion, not the earlier timeout or the next monitor tick.
				<-done
				if code := <-events; code != "session_trust_freshness_expired" || ctx.Err() != context.Canceled || time.Since(started) != duration+trustValidationMaxAge || calls != 1 {
					t.Fatal(code, ctx.Err(), time.Since(started), calls)
				}
			})
		})
	}
}

func TestSlowNegativeTrustReadStopsImmediatelyOnCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ticks := make(chan time.Time, 1)
		events := make(chan string, 3)
		done := make(chan struct{})
		go func() {
			defer close(done)
			monitorProductSession(ctx, ticks, nil, func() error {
				time.Sleep(1500 * time.Millisecond)
				return truststore.ErrPeerRevoked
			}, cancel, func(code string) { events <- code })
		}()
		started := time.Now()
		ticks <- started
		<-done
		if first, second := <-events, <-events; first != "session_trust_read_deferred" || second != "session_trust_revoked" || ctx.Err() != context.Canceled || time.Since(started) != 1500*time.Millisecond || len(events) != 0 {
			t.Fatal(first, second, ctx.Err(), time.Since(started))
		}
	})
}

func TestTrustReadSuccessAtOrAfterDeadlineCannotReviveSession(t *testing.T) {
	for _, duration := range []time.Duration{trustValidationMaxAge, trustValidationMaxAge + time.Second} {
		t.Run(duration.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ticks := make(chan time.Time, 1)
				events := make(chan string, 4)
				done, workerDone := make(chan struct{}), make(chan struct{})
				go func() {
					defer close(done)
					monitorProductSession(ctx, ticks, nil, func() error {
						defer close(workerDone)
						time.Sleep(duration)
						return nil
					}, cancel, func(code string) { events <- code })
				}()
				started := time.Now()
				ticks <- started
				<-done
				if first, second := <-events, <-events; first != "session_trust_read_deferred" || second != "session_trust_freshness_expired" || ctx.Err() != context.Canceled || time.Since(started) != trustValidationMaxAge {
					t.Fatal(first, second, ctx.Err(), time.Since(started))
				}
				<-workerDone
				synctest.Wait()
				if len(events) != 0 || ctx.Err() != context.Canceled {
					t.Fatal("late read altered the expired session")
				}
			})
		})
	}
}
