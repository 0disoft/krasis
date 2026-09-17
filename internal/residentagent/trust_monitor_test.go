package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func TestMonitoredTrustRetriesOnlyBoundedContention(t *testing.T) {
	privateErr := errors.New("private path")
	for _, tc := range []struct {
		name         string
		busy         int
		last         error
		calls, waits int
		want         error
	}{
		{"no contention", 0, nil, 1, 0, nil},
		{"short collision", 2, nil, 3, 2, nil},
		{"last attempt succeeds", 4, nil, 5, 4, nil},
		{"persistent lock", 9, nil, 5, 4, truststore.ErrStoreBusy},
		{"revoked", 0, truststore.ErrPeerRevoked, 1, 0, truststore.ErrPeerRevoked},
		{"revoked after collision", 1, truststore.ErrPeerRevoked, 2, 1, truststore.ErrPeerRevoked},
		{"I/O failure", 0, privateErr, 1, 0, privateErr},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, waits := 0, 0
			err := checkMonitoredTrust(context.Background(), func() error {
				calls++
				if calls <= tc.busy {
					return truststore.ErrStoreBusy
				}
				return tc.last
			}, func(_ context.Context, delay time.Duration) error {
				waits++
				if delay != 25*time.Millisecond {
					t.Fatal(delay)
				}
				return nil
			})
			if err != tc.want || calls != tc.calls || waits != tc.waits {
				t.Fatalf("got %v, %d checks, %d waits", err, calls, waits)
			}
		})
	}
}

func TestMonitoredTrustCancellationStopsRetry(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	err := checkMonitoredTrust(ctx, func() error { calls++; return truststore.ErrStoreBusy }, func(context.Context, time.Duration) error { cancel(); return nil })
	if err != context.Canceled || calls != 1 {
		t.Fatalf("got %v, calls %d", err, calls)
	}
}

func TestProductMonitorRecordsSafeReasonBeforeStopping(t *testing.T) {
	for _, guardExit := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		ticks := make(chan time.Time, 1)
		guard := make(chan struct{})
		want := "session_trust_check_failed"
		if guardExit {
			close(guard)
			want = "session_guard_exited"
		} else {
			ticks <- time.Now()
		}
		var codes []string
		monitorProductSession(ctx, ticks, guard, func() error { return errors.New("private path and peer") }, cancel, func(code string) {
			if ctx.Err() != nil {
				t.Fatal("reason recorded after cancellation")
			}
			codes = append(codes, code)
		})
		cancel()
		if len(codes) != 1 || codes[0] != want {
			t.Fatal(codes)
		}
	}
}
