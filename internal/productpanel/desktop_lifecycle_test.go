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
