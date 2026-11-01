package residentagent

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/truststore"
)

func TestRemoteConnectRequiresFreshInboundPermissionAndExactPeer(t *testing.T) {
	fp := strings.Repeat("a", 64)
	trust := &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Alias: "A", Fingerprint: fp}}}}
	o := &Operations{root: context.Background(), trust: trust, sessionDone: make(chan struct{}), sessionState: "CONNECTED", peerAlias: "A", sessionFingerprint: fp}
	if code := o.connectForPeer(context.Background(), fp); code != "rejected" {
		t.Fatal(code)
	}
	trust.document.Peers[0].AllowInboundControl = true
	if code := o.connectForPeer(context.Background(), fp); code != "connected" {
		t.Fatal(code)
	}
	o.sessionFingerprint = strings.Repeat("b", 64)
	if code := o.connectForPeer(context.Background(), fp); code != "busy" {
		t.Fatal("alias rebound over an active session", code)
	}
	o.sessionFingerprint = fp
	now := time.Now()
	trust.document.Peers[0].RevokedAt = &now
	if code := o.connectForPeer(context.Background(), fp); code != "rejected" {
		t.Fatal(code)
	}
	trust.document.Peers[0].RevokedAt = nil
	o.sessionState = "CONNECTING"
	o.peerAlias = "other"
	if code := o.connectForPeer(context.Background(), fp); code != "busy" {
		t.Fatal("replaced another session", code)
	}
}

func TestRemoteConnectCannotStartAfterPreflightCancellationOrPermissionChange(t *testing.T) {
	for _, revoke := range []bool{false, true} {
		fp := strings.Repeat("a", 64)
		trust := &stubTrustStore{document: truststore.Document{Peers: []truststore.PeerRecord{{Alias: "A", Fingerprint: fp, AllowInboundControl: true}}}}
		ctx, cancel := context.WithCancel(context.Background())
		o := &Operations{root: context.Background(), trust: trust, runner: func(context.Context, string, agent.SupervisorObserver) error {
			t.Error("started late session")
			return nil
		}}
		o.preflight = func(string) error {
			if revoke {
				trust.document.Peers[0].AllowInboundControl = false
			} else {
				cancel()
			}
			return nil
		}
		if code := o.connectForPeer(ctx, fp); code != "rejected" || o.sessionDone != nil {
			t.Fatal("late or unauthorized start", code)
		}
		cancel()
	}
}
