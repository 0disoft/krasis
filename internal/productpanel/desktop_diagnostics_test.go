package productpanel

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestSlowDiagnosticIsBoundedAndCannotOverwriteRecoveredState(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		unblock := make(chan struct{})
		fp := strings.Repeat("a", 64)
		client := &fakeClient{state: "FAULTED", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fp}}}
		var calls atomic.Int32
		b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, func(context.Context) error { client.state = "LOCAL"; return nil }, func(context.Context, string) error { calls.Add(1); <-unblock; return nil })
		b.recovery.Alias, b.recovery.Fingerprint = "B", fp
		now := time.Now()
		b.tick(now)
		if time.Since(now) != diagnosticWaitLimit {
			t.Fatalf("unbounded export wait: %v", time.Since(now))
		}
		b.tick(now.Add(time.Minute))
		if client.connects != 1 || calls.Load() != 1 {
			t.Fatalf("recovery blocked or extra exporter: %d %d", client.connects, calls.Load())
		}
		before := b.Refresh()
		close(unblock)
		synctest.Wait()
		after := b.Refresh()
		if after.State != before.State || after.Code != before.Code {
			t.Fatal("late diagnostic changed lifecycle state")
		}
	})
}

func TestDiagnosticFailureDoesNotBypassSafetyFailure(t *testing.T) {
	client := &fakeClient{state: "FAULTED"}
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, func(context.Context) error { return ErrAgentRecoveryFailed }, func(context.Context, string) error { return errors.New("disk full") })
	result := b.Restart()
	if result.Code != "recovery_required" || client.connects != 0 {
		t.Fatalf("safety failure hidden: %+v", result)
	}
}
