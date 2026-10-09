package residentagent

import (
	"context"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/truststore"
)

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
		if ctx.Err() != context.Canceled || len(codes) != 2 || codes[1] != "session_trust_freshness_expired" {
			t.Fatal("expiry or late-success policy changed", codes, ctx.Err())
		}
		if len(snapshots) != 2 || snapshots[0].Phase != "" || snapshots[1].Phase != "retry_wait" || snapshots[1].Elapsed != trustValidationMaxAge {
			t.Fatal("missing final blocked phase", snapshots)
		}
	})
}
