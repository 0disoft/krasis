package productpanel

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestDeferredUserConnectKeepsRequestedPeerAndHonorsLaterStops(t *testing.T) {
	for _, mode := range []string{"complete", "manual stop", "changed peer", "disconnect", "lost reply"} {
		t.Run(mode, func(t *testing.T) {
			b, c, now := newResumeTest(t, func(context.Context) error { return &inputMaintenancePending{} })
			b.updatedResume = nil
			setup, restores := "PENDING", 0
			b.PendingInputSetup = func() string { return setup }
			b.PendingInputRestore = func(context.Context) error { restores++; return nil }
			b.StartupResume = func(context.Context, BridgeSnapshot) BridgeSnapshot {
				t.Fatal("substituted login peer")
				return BridgeSnapshot{}
			}
			fp := c.peers[0].Fingerprint
			if v := b.Connect("B", fp); v.InputRefreshCode != "input_refresh_pending" || b.pendingInput == nil || b.pendingInput.alias != "B" || b.pendingInput.fingerprint != fp {
				t.Fatal("lost pending connection identity", v, b.pendingInput)
			}
			b.tick(now)
			if restores != 0 || c.connects != 0 {
				t.Fatal("connected before completion")
			}
			switch mode {
			case "manual stop":
				c.manualStop = true
			case "changed peer":
				c.peers[0].Fingerprint = strings.Repeat("c", 64)
			case "disconnect":
				b.Disconnect()
			case "lost reply":
				c.uncertain = true
			}
			setup = "READY"
			b.tick(now.Add(time.Second))
			if mode == "lost reply" {
				c.state = "LOCAL"
			}
			b.tick(now.Add(2 * time.Second))
			want := 1
			if mode == "manual stop" || mode == "changed peer" || mode == "disconnect" {
				want = 0
			}
			if c.connects != want || restores > 1 || b.pendingInput != nil {
				t.Fatal(mode, b.Refresh(), c.connects, restores)
			}
		})
	}
}

func TestInvalidDeferredConnectCannotPrepareAgent(t *testing.T) {
	b := NewDesktopLifecycle(context.Background(), nil, func(context.Context) error { t.Fatal("prepared invalid peer"); return nil }, nil)
	if b.Connect("B", strings.Repeat("z", 64)).Code != "invalid_peer" || b.pendingInput != nil {
		t.Fatal("invalid identity accepted")
	}
}
