package productpanel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"testing/synctest"

	"github.com/0disoft/krasis/internal/appsettings"
)

func TestLoginResumeSeparatesReadFailuresAndNeverReplaysConnection(t *testing.T) {
	for _, mode := range []string{"invalid", "unavailable", "transient", "missing", "manual stop", "changed", "duplicate", "lost reply", "cancelled read"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				fp := strings.Repeat("a", 64)
				value := BridgeSnapshot{State: "LOCAL", Code: "ok", Peers: []BridgePeer{{Alias: "B", Fingerprint: fp}}}
				if mode == "changed" {
					value.Peers[0].Fingerprint = strings.Repeat("b", 64)
				}
				if mode == "duplicate" {
					value.Peers = append(value.Peers, value.Peers[0])
				}
				reads, connects := 0, 0
				result := ResumeAtLogin(ctx, value, func() (appsettings.Resume, error) {
					reads++
					peer := appsettings.Resume{Alias: "B", Fingerprint: fp}
					switch mode {
					case "invalid":
						return appsettings.Resume{}, appsettings.ErrInvalidResume
					case "unavailable":
						return appsettings.Resume{}, errors.New("private path: read unavailable")
					case "transient":
						if reads < 3 {
							return appsettings.Resume{}, errors.New("sharing violation")
						}
					case "missing":
						return appsettings.Resume{}, nil
					case "manual stop":
						return appsettings.Resume{ManualStop: true}, nil
					case "cancelled read":
						cancel()
					}
					return peer, nil
				}, func(alias, fingerprint string) BridgeSnapshot {
					connects++
					if alias != "B" || fingerprint != fp {
						t.Fatal("lost pinned identity")
					}
					if mode == "lost reply" {
						return bridgeFailure("outcome_unknown")
					}
					return BridgeSnapshot{State: "CONNECTING", Code: "ok"}
				})
				wantCode, wantReads, wantConnects := "ok", 1, 0
				switch mode {
				case "invalid":
					wantCode = "startup_resume_settings_invalid"
				case "unavailable":
					wantCode, wantReads = "startup_resume_settings_unavailable", 3
				case "transient":
					wantReads, wantConnects = 3, 1
				case "changed", "duplicate":
					wantCode = "stale_peer"
				case "lost reply":
					wantCode, wantConnects = "outcome_unknown", 1
				case "cancelled read":
					wantCode = "cancelled"
				}
				if result.Code != wantCode || reads != wantReads || connects != wantConnects {
					t.Fatal(mode, result, reads, connects)
				}
			})
		})
	}
}

func TestLoginResumeFailureRemainsVisibleUntilExplicitConnection(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	b.updatedResume = nil
	for _, code := range []string{"startup_resume_settings_invalid", "startup_resume_settings_unavailable"} {
		b.save(BridgeSnapshot{State: "LOCAL", Code: code})
		b.tick(now)
		if b.Refresh().Code != code || c.connects != 0 {
			t.Fatal("login failure erased by polling", b.Refresh())
		}
	}
	if v := b.Connect("B", c.peers[0].Fingerprint); v.Code != "ok" {
		t.Fatal(v)
	}
}
