package residentagent

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func TestTrustReaderKeepsCadenceWhileObserverStallsAndRetainsRevocation(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		t.Run(map[bool]string{false: "timely-validation-chain", true: "revocation-is-terminal"}[revoked], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				started := time.Now()
				var calls atomic.Int32
				var codes []string
				monitorProductSession(ctx, ticker.C, nil, func() error {
					if calls.Add(1) == 1 {
						time.Sleep(1500 * time.Millisecond)
					}
					if revoked && time.Since(started) >= 4*time.Second {
						return truststore.ErrPeerRevoked
					}
					return nil
				}, cancel, func(code string) {
					codes = append(codes, code)
					if code == "session_trust_read_deferred" {
						time.Sleep(6 * time.Second)
					}
					if code == "session_trust_read_recovered" {
						cancel()
					}
				})
				want := "session_trust_read_recovered"
				if revoked {
					want = "session_trust_revoked"
				}
				if len(codes) != 2 || codes[1] != want || calls.Load() < 3 {
					t.Fatal("observer paused validation or lost a terminal verdict", codes, calls.Load())
				}
			})
		})
	}
}
