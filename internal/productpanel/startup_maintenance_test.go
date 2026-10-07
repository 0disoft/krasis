package productpanel

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type launchMaintenanceClient struct {
	inputMaintenanceClient
	generation int
	build      string
	missing    bool
	stops      int
}

func (c *launchMaintenanceClient) Runtime(context.Context) (localapi.RuntimeInfo, error) {
	return localapi.RuntimeInfo{Protocol: localapi.AgentProtocol, Generation: c.generation, Build: c.build, Instance: "instance"}, nil
}

func (c *launchMaintenanceClient) StopForUpgrade(context.Context, string, int, string) (localapi.OperationResult, error) {
	if c.state != "LOCAL" {
		return localapi.OperationResult{Status: "REJECTED"}, nil
	}
	c.stops++
	c.missing = true
	return localapi.OperationResult{Status: "COMPLETED", Code: "upgrade_stop_requested"}, nil
}

func TestLaunchMaintenanceDisconnectsRefreshesAndRestoresSamePeerOnce(t *testing.T) {
	for _, tc := range []struct {
		name                     string
		older                    bool
		outcome                  error
		changed                  bool
		cancel                   bool
		wantStarts, wantConnects int
	}{
		{"same agent", false, nil, false, false, 1, 1},
		{"older connected agent", true, nil, false, false, 1, 1},
		{"consent cancelled", true, ErrInputRefreshCancelled, false, false, 1, 1},
		{"installation failed", true, ErrInputRefreshFailed, false, false, 1, 1},
		{"installation pending", true, ErrInputRefreshPending, false, false, 0, 0},
		{"trust changed", true, nil, true, false, 1, 0},
		{"disconnect requested during consent", true, ErrInputRefreshCancelled, false, true, 1, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &launchMaintenanceClient{inputMaintenanceClient: inputMaintenanceClient{fakeClient: fakeClient{state: "CONNECTED", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: strings.Repeat("a", 64)}}}}, generation: 5, build: "current"}
			if tc.older {
				client.generation, client.build = 4, "previous"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			state, installs, starts, locks := "UPDATE_REQUIRED", 0, 0, 0
			refresh := &inputRefresh{inspect: func() string { return state }, run: func(context.Context) error {
				if !client.missing || client.disconnects != 1 || client.stops != 1 || locks != 1 {
					t.Fatal("installation before disconnect/owner exit")
				}
				installs++
				if tc.changed {
					client.peers[0].Fingerprint = strings.Repeat("b", 64)
				}
				if tc.cancel {
					cancel()
				}
				if tc.outcome == nil {
					state = "READY"
				}
				if tc.outcome == ErrInputRefreshPending {
					state = "PENDING"
				}
				return tc.outcome
			}}
			s := Startup{Generation: 5, Revision: "current", InputSetup: refresh.status, BeforeStart: refresh.automatic,
				Lock:   func() (io.Closer, error) { locks++; return closeLaunchLease(func() { locks-- }), nil },
				Verify: func() error { return nil }, Available: func() (bool, error) { return client.missing, nil }, Pause: func(context.Context) error { return nil },
				Open: func() (RuntimeClient, error) {
					if client.missing {
						return nil, localapi.ErrUnavailable
					}
					return client, nil
				},
				Start: func() error {
					starts++
					client.missing = false
					client.generation, client.build = 5, "current"
					return nil
				},
			}
			b := NewDesktopLifecycle(context.Background(), func() (Client, error) {
				if client.missing {
					return nil, localapi.ErrUnavailable
				}
				return client, nil
			}, s.Ensure, nil)
			b.StartupMaintenanceNeeded, b.StartupMaintenance = s.LaunchMaintenanceNeeded, s.RefreshProtectedInputAtLaunch
			value := b.initialize(ctx, NewDesktopBridge(ctx, b.open).Refresh())
			if installs != 1 || starts != tc.wantStarts || client.connects != tc.wantConnects || locks != 0 {
				t.Fatal(value, installs, starts, client.connects, locks)
			}
			if tc.outcome == ErrInputRefreshPending && value.Code != "input_refresh_pending" {
				t.Fatal(value)
			}
			if tc.outcome == ErrInputRefreshCancelled && value.InputRefreshCode != "input_refresh_cancelled" || tc.outcome == ErrInputRefreshFailed && value.InputRefreshCode != "input_refresh_failed" {
				t.Fatal("ordinary input recovery erased installation failure", value)
			}
			if !tc.cancel {
				b.tick(time.Now())
				if installs != 1 {
					t.Fatal("polling repeated installer")
				}
			}
		})
	}
}

type closeLaunchLease func()

func (f closeLaunchLease) Close() error { f(); return nil }

func TestLaunchMaintenancePreflightPreservesUnexpectedSessions(t *testing.T) {
	for _, tc := range []struct {
		name, setup, session, build    string
		generation                     int
		badBundle, wantNeeded, wantErr bool
	}{
		{"ready same build", "READY", "CONNECTED", "current", 5, false, false, false},
		{"initial installation", "NOT_INSTALLED", "CONNECTED", "current", 5, false, false, false},
		{"pending transaction", "PENDING", "CONNECTED", "current", 5, false, false, false},
		{"ready newer installation with old agent", "READY", "CONNECTED", "previous", 4, false, true, false},
		{"pairing", "UPDATE_REQUIRED", "PAIRING", "previous", 4, false, false, true},
		{"bad bundle", "UPDATE_REQUIRED", "CONNECTED", "previous", 4, true, false, true},
		{"newer agent", "UPDATE_REQUIRED", "CONNECTED", "newer", 6, false, false, true},
		{"different same generation build", "UPDATE_REQUIRED", "CONNECTED", "other", 5, false, false, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			client := &launchMaintenanceClient{inputMaintenanceClient: inputMaintenanceClient{fakeClient: fakeClient{state: tc.session}}, generation: tc.generation, build: tc.build}
			s := Startup{Generation: 5, Revision: "current", InputSetup: func() string { return tc.setup }, Open: func() (RuntimeClient, error) { return client, nil }, Verify: func() error {
				if tc.badBundle {
					return errors.New("invalid bundle")
				}
				return nil
			}}
			needed, err := s.LaunchMaintenanceNeeded(context.Background())
			if (needed && err == nil) != tc.wantNeeded || (err != nil) != tc.wantErr || client.disconnects != 0 || client.stops != 0 {
				t.Fatal(needed, err, client)
			}
		})
	}
}
