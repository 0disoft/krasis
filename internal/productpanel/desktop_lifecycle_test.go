package productpanel

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
	"strings"
	"testing"
	"time"
)

func TestDesktopLifecycleRecoveryBudgetAndIdentity(t *testing.T) {
	fp := strings.Repeat("a", 64)
	client := &fakeClient{state: "CONNECTED", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fp}}}
	starts, exports := 0, 0
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, func(context.Context) error { starts++; client.state = "LOCAL"; return nil }, func(context.Context, string) error { exports++; return nil })
	// fakeClient does not publish a peer alias; seed the known identity explicitly.
	b.recovery.Alias, b.recovery.Fingerprint = "B", fp
	client.state = "FAULTED"
	now := time.Now()
	b.tick(now) // export first, never restart before evidence
	if starts != 0 || exports != 1 {
		t.Fatal(starts, exports)
	}
	for i := 0; i < 3; i++ {
		b.tick(now.Add(time.Duration(i+1) * time.Minute))
		client.state = "FAULTED"
	}
	b.tick(now.Add(10 * time.Minute))
	if starts != 3 || client.connects != 3 || b.Refresh().Code != "recovery_exhausted" {
		t.Fatal(starts, client.connects, b.Refresh())
	}
}

func TestDesktopLifecycleDiagnosticFailureDoesNotBlockVerifiedRecovery(t *testing.T) {
	fp := strings.Repeat("a", 64)
	client := &fakeClient{state: "FAULTED", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fp}}}
	starts := 0
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, func(context.Context) error { starts++; client.state = "LOCAL"; return nil }, func(context.Context, string) error { return errors.New("private path") })
	b.recovery.Alias, b.recovery.Fingerprint = "B", strings.Repeat("a", 64)
	b.tick(time.Now())
	b.tick(time.Now().Add(time.Minute))
	if starts != 1 || client.connects != 1 || b.Refresh().Code != "diagnostic_failed" {
		t.Fatal(starts, b.Refresh())
	}
}

func TestDesktopLifecycleDisconnectCancelsRecovery(t *testing.T) {
	client := &fakeClient{state: "FAULTED"}
	entered := make(chan struct{})
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, func(ctx context.Context) error {
		close(entered)
		<-ctx.Done()
		client.state = "LOCAL"
		return ctx.Err()
	}, func(context.Context, string) error { return nil })
	b.recovery.Alias, b.recovery.Fingerprint = "B", strings.Repeat("a", 64)
	now := time.Now()
	b.tick(now)
	done := make(chan struct{})
	go func() { b.tick(now.Add(time.Minute)); close(done) }()
	<-entered
	result := b.Disconnect()
	<-done
	if result.State != "LOCAL" || client.connects != 0 {
		t.Fatal(result, client.connects)
	}
	b.tick(now.Add(2 * time.Minute))
	if client.connects != 0 {
		t.Fatal("reconnected after disconnect")
	}
}

func TestDesktopLifecycleDoesNotReconnectChangedIdentity(t *testing.T) {
	client := &fakeClient{state: "FAULTED", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: strings.Repeat("b", 64)}}}
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, func(context.Context) error { client.state = "LOCAL"; return nil }, func(context.Context, string) error { return nil })
	b.recovery.Alias, b.recovery.Fingerprint = "B", strings.Repeat("a", 64)
	now := time.Now()
	b.tick(now)
	b.tick(now.Add(time.Minute))
	if client.connects != 0 || b.Refresh().Code != "stale_peer" {
		t.Fatal(client.connects, b.Refresh())
	}
}

func TestDesktopLifecycleWorkerStopsWithOwner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	entered, done := make(chan struct{}), make(chan struct{})
	starts := 0
	b := NewDesktopLifecycle(ctx, func() (Client, error) { return &fakeClient{state: "LOCAL"}, nil }, func(ctx context.Context) error { starts++; close(entered); <-ctx.Done(); return ctx.Err() }, nil)
	go func() { b.Run(); close(done) }()
	<-entered
	cancel()
	<-done
	if starts != 1 {
		t.Fatal(starts)
	}
}

func TestDesktopLifecycleKeepsUpgradeBusyNoticeWhileAgentRemainsConnected(t *testing.T) {
	client := &fakeClient{state: "CONNECTED"}
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, nil, nil)
	b.failedStartup(context.Background(), ErrAgentBusy)
	b.tick(time.Now())
	if b.Refresh().Code != "agent_busy" {
		t.Fatal("upgrade refusal disappeared while old agent remained connected")
	}
}

func TestRecoveryHonorsCLIStopDuringPreparation(t *testing.T) {
	for _, display := range []bool{false, true} {
		b, c, now := newResumeTest(t, nil)
		b.updatedResume = nil
		b.recovery.Alias, b.recovery.Fingerprint = "B", c.peers[0].Fingerprint
		c.state = "FAULTED"
		if display {
			c.state, c.stopReason = "LOCAL", "display_change"
		}
		b.ensure = func(context.Context) error {
			c.state, c.stopReason, c.manualStop = "LOCAL", "", true
			return nil
		}
		b.tick(now)
		b.tick(now.Add(6 * time.Second))
		if c.connects != 0 || b.Refresh().Code != "cancelled" || !b.recovery.blocked {
			t.Fatal("automatic recovery undid CLI stop", display, b.Refresh(), c.connects)
		}
		b.tick(now.Add(12 * time.Second))
		if c.connects != 0 {
			t.Fatal("canceled recovery retried")
		}
		if value := b.Connect("B", c.peers[0].Fingerprint); value.Code != "ok" || c.connects != 1 {
			t.Fatal("explicit connect blocked", value)
		}
	}
}

func TestDisplayRecoveryPreservesIdentityAndDeadlineDuringTrustFailure(t *testing.T) {
	for _, expired := range []bool{false, true} {
		fp := strings.Repeat("a", 64)
		c := &fakeClient{state: "LOCAL", stopReason: "display_change", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fp}}}
		starts := 0
		b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return c, nil }, func(context.Context) error { starts++; return nil }, nil)
		b.recovery.Alias, b.recovery.Fingerprint = "B", fp
		now := time.Now()
		b.tick(now)
		deadlineStart := b.recovery.displaySince
		c.trustErr = errors.New("temporarily unreadable trust")
		b.tick(now.Add(4 * time.Second))
		if b.recovery.Alias != "B" || b.recovery.Fingerprint != fp || b.recovery.displaySince != deadlineStart || starts != 0 || c.connects != 0 {
			t.Fatal("unavailable read destroyed or executed recovery", b.recovery, starts, c.connects)
		}
		c.trustErr = nil
		elapsed := 5 * time.Second
		if expired {
			elapsed = 31 * time.Second
		}
		b.tick(now.Add(elapsed))
		if expired {
			if c.connects != 0 || b.Refresh().Code != "display_recovery_expired" {
				t.Fatal("unavailable reads renewed display budget", b.Refresh())
			}
		} else if c.connects != 1 || starts != 1 {
			t.Fatal("recovered read failed to resume", b.Refresh(), c.connects, starts)
		}
	}
}

func TestDisplayRecoveryStillDropsVerifiedChangedIdentity(t *testing.T) {
	fp := strings.Repeat("a", 64)
	c := &fakeClient{state: "LOCAL", stopReason: "display_change", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fp}}}
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return c, nil }, func(context.Context) error { t.Fatal("prepared changed peer"); return nil }, nil)
	b.recovery.Alias, b.recovery.Fingerprint = "B", fp
	c.peers[0].Fingerprint = strings.Repeat("b", 64)
	b.tick(time.Now())
	if b.recovery.Alias != "" || c.connects != 0 {
		t.Fatal("verified identity change retained", b.recovery)
	}
}

func TestDesktopLifecycleRetriesDisplayChangeAfterGeometrySettles(t *testing.T) {
	fp := strings.Repeat("a", 64)
	client := &fakeClient{state: "LOCAL", stopReason: "display_change", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fp}}}
	ready := false
	starts := 0
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, func(context.Context) error { starts++; return nil }, nil, func() bool { return ready })
	b.recovery.Alias, b.recovery.Fingerprint = "B", fp
	now := time.Now()
	b.tick(now)
	b.tick(now.Add(4 * time.Second))
	if starts != 0 || client.connects != 0 {
		t.Fatal("connected before stable geometry")
	}
	ready = true
	b.tick(now.Add(5 * time.Second))
	if starts != 0 || client.connects != 0 {
		t.Fatal("connected before stable display window")
	}
	b.tick(now.Add(8 * time.Second))
	if starts != 1 || client.connects != 1 || b.Refresh().State != "CONNECTING" {
		t.Fatal(starts, client.connects, b.Refresh())
	}
	client.state, client.stopReason = "LOCAL", "display_change"
	b.tick(now.Add(10 * time.Second))
	if client.connects != 1 {
		t.Fatal("retried before backoff")
	}
	b.tick(now.Add(14 * time.Second))
	if starts != 2 || client.connects != 2 {
		t.Fatal("failed to retry transient display reconnect", starts, client.connects)
	}
}
