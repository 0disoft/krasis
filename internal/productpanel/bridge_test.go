package productpanel

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

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
