package residentagent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func TestTrustExpiryDistinguishesQueuedOutcomeWithoutRevivingAuthority(t *testing.T) {
	for _, rejected := range []bool{false, true} {
		for _, late := range []bool{false, true} {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithDeadline(context.Background(), time.Now())
				defer cancel()
				deadline := time.Now()
				completed := trustCheckCompletion{finished: deadline.Add(-time.Millisecond)}
				want := "trust_expiry_result_ok"
				if rejected {
					completed.err = errors.New("private trust error")
					want = "trust_expiry_result_error"
				}
				if late {
					completed.finished = deadline
					want += "_late"
				} else {
					want += "_before_deadline"
				}
				ready := make(chan trustCheckCompletion, 1)
				ready <- completed
				time.Sleep(2 * time.Second)
				var codes []string
				recordTrustExpiry(deadline, ready, nil, func(code string) { codes = append(codes, code) })
				if len(codes) != 2 || codes[0] != want || codes[1] != "trust_expiry_late_ge1s" || ctx.Err() != context.DeadlineExceeded {
					t.Fatal("queued result changed deadline or lost verdict", codes, ctx.Err())
				}
			})
		}
	}
}

func TestTrustExpiryNeverWaitsForUnstartedOrBlockedRead(t *testing.T) {
	for _, active := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			trace := truststore.NewReadTrace()
			want := "trust_expiry_not_started"
			if active {
				trace.EnterRetryWait()
				want = "trust_expiry_reading"
			}
			started := time.Now()
			var codes []string
			recordTrustExpiry(started, make(chan trustCheckCompletion), trace, func(code string) { codes = append(codes, code) })
			if time.Since(started) != 0 || len(codes) != 2 || codes[0] != want || codes[1] != "trust_expiry_late_lt1ms" {
				t.Fatal("terminal observation waited for read", codes, time.Since(started))
			}
		})
	}
}
