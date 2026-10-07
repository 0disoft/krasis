package productpanel

import (
	"context"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/localapi"
)

func TestStartupRestoredAgentPromotesReadinessAndHonorsLoginResumeGuards(t *testing.T) {
	for _, mode := range []string{"failed", "cancelled", "restored busy", "pre-stop busy", "pending", "manual stop", "changed identity", "lost reply", "cancelled startup"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			fp := strings.Repeat("a", 64)
			c := &inputMaintenanceClient{fakeClient: fakeClient{state: "LOCAL"}}
			c.peers = append(c.peers, localapi.TrustedPeer{Alias: "B", Fingerprint: fp})
			b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return c, nil }, nil, nil)
			b.StartupMaintenanceNeeded = func(context.Context) (bool, error) { return true, nil }
			outcome, wantCode := ErrInputRefreshFailed, "input_refresh_failed"
			if mode == "cancelled" {
				outcome, wantCode = ErrInputRefreshCancelled, "input_refresh_cancelled"
			}
			if mode == "restored busy" || mode == "pre-stop busy" {
				outcome, wantCode = ErrStartupBusy, "busy"
			}
			if mode == "pending" {
				outcome, wantCode = ErrInputRefreshPending, "input_refresh_pending"
			}
			b.StartupMaintenance = func(context.Context) error {
				if mode == "cancelled startup" {
					cancel()
				}
				if mode == "pre-stop busy" || mode == "pending" {
					return outcome
				}
				return &inputMaintenanceRestored{cause: outcome}
			}
			promotions, callbacks, reads, connects := 0, 0, 0, 0
			b.StartupReady = func() { promotions++ }
			b.StartupResume = func(ctx context.Context, value BridgeSnapshot) BridgeSnapshot {
				callbacks++
				return ResumeAtLogin(ctx, value, func() (appsettings.Resume, error) {
					reads++
					peer := appsettings.Resume{Alias: "B", Fingerprint: fp, ManualStop: mode == "manual stop"}
					if mode == "changed identity" {
						peer.Fingerprint = strings.Repeat("b", 64)
					}
					return peer, nil
				}, func(alias, fingerprint string) BridgeSnapshot {
					connects++
					if alias != "B" || fingerprint != fp {
						t.Fatal("unpinned login resume")
					}
					if mode == "lost reply" {
						return bridgeFailure("outcome_unknown")
					}
					return BridgeSnapshot{Code: "ok", State: "CONNECTING"}
				})
			}
			value := b.initialize(ctx, NewDesktopBridge(ctx, b.open).Refresh())
			wantPrepared, wantConnects := 1, 1
			if mode == "pre-stop busy" || mode == "pending" || mode == "cancelled startup" {
				wantPrepared, wantConnects = 0, 0
			}
			if mode == "manual stop" || mode == "changed identity" {
				wantConnects = 0
			}
			if promotions != wantPrepared || callbacks != wantPrepared || reads != wantPrepared || connects != wantConnects {
				t.Fatal(value, promotions, callbacks, reads, connects)
			}
			if value.InputRefreshCode != wantCode || value.ReconnectCode != "not_attempted" {
				t.Fatal("startup resume erased installer result", value)
			}
			if mode == "lost reply" && value.Code != "outcome_unknown" || mode == "changed identity" && value.Code != "stale_peer" {
				t.Fatal("resume failure hidden", value)
			}
		})
	}
}
