package productpanel

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

// OpenClient can read a saved endpoint even after its process has exited.
type staleEndpointClient struct {
	inputRuntimeStub
	err error
}

func (c *staleEndpointClient) Runtime(ctx context.Context) (localapi.RuntimeInfo, error) {
	if c.err != nil {
		return localapi.RuntimeInfo{}, c.err
	}
	return c.inputRuntimeStub.Runtime(ctx)
}

func (c *staleEndpointClient) Status(ctx context.Context) (localapi.Status, error) {
	if c.err != nil {
		return localapi.Status{}, c.err
	}
	return c.inputRuntimeStub.Status(ctx)
}

func TestMaintenancePreflightWithSavedUnavailableEndpoint(t *testing.T) {
	for _, response := range []error{localapi.ErrUnavailable, localapi.ErrTransport, localapi.ErrAuthentication, localapi.ErrProtocol} {
		for _, setup := range []string{"READY", "UPDATE_REQUIRED"} {
			t.Run(response.Error()+"/"+setup, func(t *testing.T) {
				c := &staleEndpointClient{err: response}
				verified := 0
				s := Startup{Open: func() (RuntimeClient, error) { return c, nil }, InputSetup: func() string { return setup }, Verify: func() error { verified++; return nil }}
				needed, err := s.LaunchMaintenanceNeeded(context.Background())
				absent := errors.Is(response, localapi.ErrUnavailable) || errors.Is(response, localapi.ErrTransport)
				if !absent {
					if !errors.Is(err, response) || needed || verified != 0 {
						t.Fatal(needed, err, verified)
					}
					return
				}
				want := setup == "UPDATE_REQUIRED"
				if err != nil || needed != want || (verified == 1) != want {
					t.Fatal(needed, err, verified)
				}
			})
		}
	}
}

func TestInputMaintenanceWithSavedUnavailableEndpoint(t *testing.T) {
	for _, response := range []error{localapi.ErrUnavailable, localapi.ErrTransport, localapi.ErrAuthentication, localapi.ErrProtocol} {
		t.Run(response.Error(), func(t *testing.T) {
			c := &staleEndpointClient{inputRuntimeStub: inputRuntimeStub{runtimeStub: runtimeStub{generation: 4}, build: strings.Repeat("a", 40)}, err: response}
			available, probes, installs, starts, leases := false, 0, 0, 0, 0
			s := Startup{Generation: 4, Revision: c.build,
				Open: func() (RuntimeClient, error) { return c, nil }, Verify: func() error { return nil },
				Lock:      func() (io.Closer, error) { leases++; return closeLaunchLease(func() { leases-- }), nil },
				Available: func() (bool, error) { probes++; return available, nil },
				Pause:     func(context.Context) error { available = true; return nil },
				InputRefresh: func(context.Context) error {
					if !available || probes < 2 || leases != 1 {
						t.Fatal("installer ran before verified owner exit")
					}
					installs++
					return nil
				},
				Start: func() error { starts++; c.err = nil; return nil },
			}
			s.BeforeStart = s.InputRefresh
			err := s.RefreshProtectedInputAtLaunch(context.Background())
			absent := errors.Is(response, localapi.ErrUnavailable) || errors.Is(response, localapi.ErrTransport)
			if absent {
				if err != nil || installs != 1 || starts != 1 || c.stops != 0 || leases != 0 {
					t.Fatal(err, installs, starts, c.stops, leases)
				}
			} else if !errors.Is(err, response) || probes != 0 || installs != 0 || starts != 0 || leases != 0 {
				t.Fatal("unverified response triggered installation", err, probes, installs, starts, leases)
			}
		})
	}
}
