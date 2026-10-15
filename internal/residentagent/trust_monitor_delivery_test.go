package residentagent

import (
	"context"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

// Delay the observer, not the file check: a verdict completed at 2s is queued
// before the original 5s deadline. Observation at 5.25s must retain that exact
// verdict; it is not a read that completed after expiry.
func TestTrustMonitorRetainsTimelyVerdictWhenObserverWakesAfterOldDeadline(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		t.Run(map[bool]string{false: "valid", true: "revoked"}[rejected], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ticks := make(chan time.Time, 1)
				ticks <- time.Now()
				var codes []string
				monitorProductSession(ctx, ticks, nil, func() error {
					time.Sleep(2 * time.Second)
					if rejected {
						return truststore.ErrPeerRevoked
					}
					return nil
				}, cancel, func(code string) {
					codes = append(codes, code)
					if code == "session_trust_read_deferred" {
						time.Sleep(4250 * time.Millisecond)
					}
					if code == "session_trust_read_recovered" {
						cancel()
					}
				})
				want := []string{"session_trust_read_deferred", "session_trust_read_recovered"}
				if rejected {
					want[1] = "session_trust_revoked"
				}
				if !reflect.DeepEqual(codes, want) {
					t.Fatalf("queued verdict lost: got %v want %v", codes, want)
				}
			})
		})
	}
}

func TestTrustMonitorNeverExtendsFreshnessFromDelayedObservation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		ticks := make(chan time.Time, 1)
		ticks <- time.Now()
		var codes []string
		monitorProductSession(ctx, ticks, nil, func() error {
			time.Sleep(2 * time.Second)
			return nil
		}, cancel, func(code string) {
			codes = append(codes, code)
			if code == "session_trust_read_deferred" {
				time.Sleep(7 * time.Second)
			}
		})
		if want := []string{"session_trust_read_deferred", "session_trust_freshness_expired"}; !reflect.DeepEqual(codes, want) {
			t.Fatalf("stale completion renewed authority: %v", codes)
		}
	})
}
