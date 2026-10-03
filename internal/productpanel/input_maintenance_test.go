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
