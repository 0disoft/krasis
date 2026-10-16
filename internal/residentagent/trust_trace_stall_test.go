package residentagent

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

func TestTrustTraceStallCannotExpireANewerValidationOrHideRevocation(t *testing.T) {
	for _, revoked := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh-chain", true: "revocation"}[revoked], func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				ticker := time.NewTicker(time.Second)
				defer ticker.Stop()
				var calls atomic.Int32
				var codes []string
				traceReturned := make(chan struct{}, 1)
				done := make(chan struct{})
				go func() {
					defer close(done)
					stalled := false
					monitorProductSessionObserved(ctx, ticker.C, nil, func(*truststore.ReadTrace) error {
						n := calls.Add(1)
						if n == 1 {
							time.Sleep(60 * time.Millisecond)
						}
						if revoked && n >= 3 {
							return truststore.ErrPeerRevoked
						}
						return nil
					}, cancel, func(code string) { codes = append(codes, code) }, func(truststore.ReadTraceSnapshot) {
						if !stalled {
							stalled = true
							time.Sleep(6 * time.Second)
							traceReturned <- struct{}{}
						}
					})
				}()
				<-traceReturned
				synctest.Wait()
				if revoked {
					if !slices.Contains(codes, "session_trust_revoked") {
						t.Fatal("revocation lost behind diagnostics", codes)
					}
				} else if ctx.Err() != nil || calls.Load() < 5 {
					t.Fatal("stale observer snapshot expired a continuously validated session", calls.Load(), codes)
				}
				cancel()
				<-done
			})
		})
	}
}
