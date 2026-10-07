package productpanel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

type inputRuntimeStub struct {
	runtimeStub
	build string
}

func (r *inputRuntimeStub) Runtime(ctx context.Context) (localapi.RuntimeInfo, error) {
	v, e := r.runtimeStub.Runtime(ctx)
	v.Build = r.build
	return v, e
}

func TestInputMaintenanceWaitsForExitAndRestartsAfterTerminalOutcome(t *testing.T) {
	for _, outcome := range []error{nil, ErrInputRefreshCancelled, ErrInputRefreshFailed, ErrInputRefreshPending} {
		t.Run(strings.ReplaceAll(fmt.Sprint(outcome), " ", "_"), func(t *testing.T) {
			r := &inputRuntimeStub{runtimeStub: runtimeStub{generation: 4}, build: strings.Repeat("a", 40)}
			starts, probes, installs := 0, 0, 0
			missing := false
			s := Startup{Generation: 4, Revision: r.build, Lock: func() (io.Closer, error) { return closerStub{}, nil }, Verify: func() error { return nil },
				Open: func() (RuntimeClient, error) {
					if missing {
						return nil, localapi.ErrUnavailable
					}
					return r, nil
				},
				Available: func() (bool, error) {
					probes++
					if r.stops > 0 {
						missing = true
					}
					return probes > 2, nil
				}, Pause: func(context.Context) error { return nil },
				InputRefresh: func(context.Context) error {
					if probes <= 2 || r.stops != 1 {
						t.Fatal("installation before owner exit")
					}
					installs++
					return outcome
				},
				Start: func() error { starts++; missing = false; return nil }}
			err := s.RefreshProtectedInput(context.Background())
			wantStarts := 1
			if errors.Is(outcome, ErrInputRefreshPending) {
				wantStarts = 0
			}
			if err != outcome || installs != 1 || starts != wantStarts {
				t.Fatalf("err=%v installs=%d starts=%d", err, installs, starts)
			}
		})
	}
}

func TestInputMaintenanceDoesNotStopAnUnexpectedAgent(t *testing.T) {
	r := &inputRuntimeStub{runtimeStub: runtimeStub{generation: 5}, build: strings.Repeat("a", 40)}
	s := Startup{Generation: 4, Revision: r.build, Lock: func() (io.Closer, error) { return closerStub{}, nil }, Verify: func() error { return nil }, Open: func() (RuntimeClient, error) { return r, nil }, InputRefresh: func(context.Context) error { t.Fatal("installer invoked"); return nil }}
	if !errors.Is(s.RefreshProtectedInput(context.Background()), ErrAgentIncompatible) || r.stops != 0 {
		t.Fatal("unexpected agent stopped")
	}
}

type inputMaintenanceClient struct{ fakeClient }

func (c *inputMaintenanceClient) Status(ctx context.Context) (localapi.Status, error) {
	v, e := c.fakeClient.Status(ctx)
	if v.State == "CONNECTED" {
		v.PeerAlias = "B"
	}
	return v, e
}

func TestInputMaintenancePreservesSessionWhenPeerCannotBeConfirmed(t *testing.T) {
	for _, mode := range []string{"trust unavailable", "missing fingerprint", "duplicate", "revoked", "pairing"} {
		t.Run(mode, func(t *testing.T) {
			b, c, _ := newResumeTest(t, nil)
			b.updatedResume = nil
			c.state = "CONNECTED"
			switch mode {
			case "trust unavailable":
				c.trustErr = errors.New("trust temporarily unreadable")
			case "missing fingerprint":
				c.peers[0].Fingerprint = ""
			case "duplicate":
				c.peers = append(c.peers, c.peers[0])
			case "revoked":
				c.peers[0].Revoked = true
			case "pairing":
				c.state = "PAIRING"
			}
			before := c.state
			b.recovery.Alias = "B"
			value := b.RefreshProtectedInput(func(context.Context) error { t.Fatal("installed without confirmed session identity"); return nil })
			if c.disconnects != 0 || c.connects != 0 || c.state != before || value.Code == "ok" || b.recovery.Alias != "B" {
				t.Fatal("preflight changed session or recovery", value, c.fakeClient, b.recovery)
			}
		})
	}
}

func TestInputMaintenanceAllowsVerifiedIdleStartup(t *testing.T) {
	c := &fakeClient{state: "LOCAL"}
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return c, nil }, nil, nil)
	installs := 0
	value := b.RefreshProtectedInput(func(context.Context) error { installs++; return nil })
	if value.Code != "ok" || installs != 1 || c.disconnects != 0 || c.connects != 0 {
		t.Fatal(value, installs, c)
	}
}

func TestInputMaintenanceHonorsCLIStopBeforeResume(t *testing.T) {
	b, c, _ := newResumeTest(t, nil)
	b.updatedResume = nil
	c.state = "CONNECTED"
	value := b.RefreshProtectedInput(func(context.Context) error {
		c.manualStop = true // CLI stop during installation, after the temporary pause.
		return nil
	})
	if c.disconnects != 1 || c.connects != 0 || value.Code != "cancelled" || !b.recovery.blocked {
		t.Fatal(value, c.connects, c.disconnects)
	}
}

func TestInputMaintenanceReconnectsOnlyTheSameTrustedPeerAfterTerminalOutcome(t *testing.T) {
	for _, tc := range []struct {
		name     string
		outcome  error
		changed  bool
		connects int
	}{
		{"complete", nil, false, 1}, {"cancelled", ErrInputRefreshCancelled, false, 1}, {"failed", ErrInputRefreshFailed, false, 1},
		{"pending", ErrInputRefreshPending, false, 0}, {"identity changed", nil, true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &inputMaintenanceClient{fakeClient: fakeClient{state: "CONNECTED", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: strings.Repeat("a", 64)}}}}
			b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, nil, nil)
			value := b.RefreshProtectedInput(func(context.Context) error {
				if client.state != "LOCAL" || client.disconnects != 1 {
					t.Fatal("input not disconnected before installation")
				}
				if tc.changed {
					client.peers[0].Fingerprint = strings.Repeat("b", 64)
				}
				return tc.outcome
			})
			if client.connects != tc.connects || value.Code == "ok" && tc.outcome != nil {
				t.Fatal(value, client.connects)
			}
		})
	}
}

func TestInputMaintenanceKeepsRefreshAndReconnectOutcomesSeparate(t *testing.T) {
	for _, tc := range []struct {
		name, refreshCode, reconnectCode, code string
		outcome                                error
		uncertain, manualStop, changed         bool
		connects                               int
	}{
		{"success", "ok", "ok", "ok", nil, false, false, false, 1},
		{"cancelled with accepted resume", "input_refresh_cancelled", "ok", "input_refresh_cancelled", ErrInputRefreshCancelled, false, false, false, 1},
		{"failed with lost response", "input_refresh_failed", "outcome_unknown", "outcome_unknown", ErrInputRefreshFailed, true, false, false, 1},
		{"cancelled with manual stop", "input_refresh_cancelled", "cancelled", "cancelled", ErrInputRefreshCancelled, false, true, false, 0},
		{"failed with changed peer", "input_refresh_failed", "stale_peer", "stale_peer", ErrInputRefreshFailed, false, false, true, 0},
		{"pending without resume", "input_refresh_pending", "not_attempted", "input_refresh_pending", ErrInputRefreshPending, false, false, false, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			b, c, now := newResumeTest(t, nil)
			b.updatedResume = nil
			c.state = "CONNECTED"
			value := b.RefreshProtectedInput(func(context.Context) error {
				c.uncertain, c.manualStop = tc.uncertain, tc.manualStop
				if tc.changed {
					c.peers[0].Fingerprint = strings.Repeat("c", 64)
				}
				return tc.outcome
			})
			if value.InputRefreshCode != tc.refreshCode || value.ReconnectCode != tc.reconnectCode || value.Code != tc.code || c.connects != tc.connects {
				t.Fatal("maintenance overwrote reconnect outcome", value, c.connects)
			}
			b.tick(now)
			value = b.Refresh()
			if value.InputRefreshCode != tc.refreshCode || value.ReconnectCode != tc.reconnectCode || c.connects != tc.connects {
				t.Fatal("poll erased outcome or repeated connect", value, c.connects)
			}
			value = b.Disconnect()
			if value.InputRefreshCode != "" || value.ReconnectCode != "" {
				t.Fatal("explicit disconnect retained old maintenance outcome", value)
			}
		})
	}
}

func TestExplicitConnectionClearsPreviousMaintenanceOutcome(t *testing.T) {
	b, c, _ := newResumeTest(t, nil)
	b.updatedResume = nil
	c.state = "CONNECTED"
	b.RefreshProtectedInput(func(context.Context) error { return ErrInputRefreshFailed })
	c.state = "LOCAL" // A subsequent CLI disconnect does not clear the UI outcome.
	value := b.Connect("B", c.peers[0].Fingerprint)
	if value.Code != "ok" || value.InputRefreshCode != "" || value.ReconnectCode != "" {
		t.Fatal("explicit connect retained old maintenance outcome", value)
	}
}
