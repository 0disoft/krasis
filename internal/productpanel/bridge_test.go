package productpanel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type interruptedBridgeClient struct {
	*resumeTestClient
	phase string
	stop  context.CancelFunc
}

func (c *interruptedBridgeClient) interrupt(ctx context.Context) error {
	if c.stop != nil {
		c.stop()
	}
	<-ctx.Done()
	return ctx.Err()
}

func (c *interruptedBridgeClient) Status(ctx context.Context) (localapi.Status, error) {
	if c.phase == "before" || c.phase == "after" && c.connects > 0 {
		return localapi.Status{}, c.interrupt(ctx)
	}
	return c.resumeTestClient.Status(ctx)
}

func (c *interruptedBridgeClient) ResumeConnection(ctx context.Context, alias string) (localapi.OperationResult, error) {
	result, err := c.resumeTestClient.ResumeConnection(ctx, alias)
	if c.phase == "command" {
		return localapi.OperationResult{}, c.interrupt(ctx)
	}
	return result, err
}

func TestBridgeDistinguishesTimeoutCancellationAndUnknownMutation(t *testing.T) {
	for _, phase := range []string{"before", "command", "after"} {
		for _, userCancel := range []bool{false, true} {
			synctest.Test(t, func(t *testing.T) {
				_, base, _ := newResumeTest(t, nil)
				c := &interruptedBridgeClient{resumeTestClient: base, phase: phase}
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				if userCancel {
					c.stop = cancel
				}
				b := NewDesktopBridge(ctx, func() (Client, error) { return c, nil })
				value := b.Resume("B", base.peers[0].Fingerprint)
				want, calls := "outcome_unknown", 1
				if phase == "before" {
					want, calls = "unavailable", 0
					if userCancel {
						want = "cancelled"
					}
				}
				if value.Code != want || base.connects != calls {
					t.Fatal(phase, userCancel, value, base.connects)
				}
			})
		}
	}
}

func TestUpdateResumeObservesTimedOutMutationWithoutReplayingIt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		b, base, now := newResumeTest(t, nil)
		c := &interruptedBridgeClient{resumeTestClient: base, phase: "command"}
		b.open = func() (Client, error) { return c, nil }
		b.tick(now)
		if b.Refresh().Code != "outcome_unknown" || b.updatedResume == nil || !b.updatedResume.uncertain || base.connects != 1 {
			t.Fatal("timeout lost mutation uncertainty", b.Refresh(), base.connects)
		}
		base.state = "LOCAL"
		b.tick(now.Add(16 * time.Second))
		if base.connects != 1 || b.Refresh().Code != "outcome_unknown" {
			t.Fatal("timeout replayed connection", b.Refresh(), base.connects)
		}
	})
}

func TestDesktopBridgeReadsFreshStateAndIdentity(t *testing.T) {
	fp := strings.Repeat("a", 64)
	c := &fakeClient{state: "LOCAL", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fp}}}
	opens := 0
	b := NewDesktopBridge(context.Background(), func() (Client, error) { opens++; return c, nil })
	if r := b.Refresh(); r.Schema != BridgeSchema || len(r.Peers) != 1 || r.State != "LOCAL" {
		t.Fatalf("refresh: %+v", r)
	}
	if r := b.Connect("B", strings.Repeat("b", 64)); r.Code != "stale_peer" || c.connects != 0 {
		t.Fatalf("stale: %+v", r)
	}
	if r := b.Connect("B", fp); r.State != "CONNECTING" || c.connects != 1 {
		t.Fatalf("connect: %+v", r)
	}
	c.trustErr = errors.New("damaged trust")
	if r := b.Disconnect(); r.State != "LOCAL" || c.disconnects != 1 {
		t.Fatalf("disconnect: %+v", r)
	}
	if opens != 4 {
		t.Fatalf("credential opener called %d times", opens)
	}
}

func TestDesktopBridgeReportsTrustFailureWithoutLosingSessionMetadata(t *testing.T) {
	client := &resumeTestClient{fakeClient: &fakeClient{state: "CONNECTED", stopReason: "display_change", trustErr: errors.New("private trust path")}}
	b := NewDesktopBridge(context.Background(), func() (Client, error) { return client, nil })
	v := b.Refresh()
	if v.State != "CONNECTED" || v.PeerAlias != "B" || v.StopReason != "display_change" || v.Code != "unavailable" || len(v.Peers) != 0 {
		t.Fatal(v)
	}
	if v = b.Connect("B", strings.Repeat("b", 64)); v.Code != "unavailable" || client.connects != 0 {
		t.Fatal(v, client.connects)
	}
}

func TestDesktopBridgeDoesNotRetainProtectedModeAfterFallback(t *testing.T) {
	c := &fakeClient{state: "CONNECTED", inputBackend: "PROTECTED"}
	b := NewDesktopBridge(context.Background(), func() (Client, error) { return c, nil })
	for _, mode := range []string{"PROTECTED", "PORTABLE", "", "private-path"} {
		c.inputBackend = mode
		want := mode
		if mode != "PROTECTED" && mode != "PORTABLE" {
			want = "UNKNOWN"
		}
		if r := b.Refresh(); r.InputBackend != want || r.State != "CONNECTED" {
			t.Fatalf("mode=%s result=%+v", mode, r)
		}
	}
}

func TestDesktopBridgeFailsClosedAndRedacts(t *testing.T) {
	b := NewDesktopBridge(context.Background(), func() (Client, error) { return nil, errors.New("private bearer and path") })
	raw, _ := json.Marshal(b.Refresh())
	if strings.Contains(string(raw), "private") || !strings.Contains(string(raw), `"peers":[]`) {
		t.Fatalf("unsafe result %s", raw)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	b = NewDesktopBridge(ctx, func() (Client, error) { t.Fatal("opened after shutdown"); return nil, nil })
	if r := b.Refresh(); r.Code != "cancelled" {
		t.Fatalf("cancel: %+v", r)
	}
	var zero DesktopBridge
	if r := zero.Refresh(); r.Code != "unavailable" {
		t.Fatalf("zero: %+v", r)
	}
}

func TestDesktopBridgeRejectsOverlappingAction(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	b := NewDesktopBridge(context.Background(), func() (Client, error) {
		close(entered)
		<-release
		return &fakeClient{state: "LOCAL"}, nil
	})
	done := make(chan BridgeSnapshot, 1)
	go func() { done <- b.Refresh() }()
	<-entered
	busy := b.Disconnect()
	close(release)
	<-done
	if busy.Code != "busy" {
		t.Fatalf("overlap: %+v", busy)
	}
}

type bridgeRejectClient struct{ *fakeClient }

func (c bridgeRejectClient) Connect(context.Context, string) (localapi.OperationResult, error) {
	return localapi.OperationResult{Status: "REJECTED"}, nil
}

func TestDesktopBridgeDoesNotReportRejectedOrFaultedAsSuccess(t *testing.T) {
	fp := strings.Repeat("a", 64)
	c := &fakeClient{state: "LOCAL", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fp}}}
	b := NewDesktopBridge(context.Background(), func() (Client, error) { return bridgeRejectClient{c}, nil })
	if r := b.Connect("B", fp); r.Code != "rejected" || r.State != "LOCAL" {
		t.Fatalf("rejected: %+v", r)
	}
	if r := b.Connect("B", strings.Repeat("z", 64)); r.Code != "invalid_peer" {
		t.Fatalf("invalid: %+v", r)
	}
	c.state = "FAULTED"
	if r := b.Disconnect(); r.Code != "recovery_required" || c.disconnects != 0 {
		t.Fatalf("fault: %+v", r)
	}
}
