//go:build windows

package appinstall

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/localapi"
)

type shutdownLease struct{ closed bool }

func (l *shutdownLease) Close() error { l.closed = true; return nil }

type shutdownAgentStub struct {
	info                                 localapi.RuntimeInfo
	state                                string
	paused, stopped                      localapi.OperationResult
	identityError, pauseError, stopError error
	guard                                bool
	events                               *[]string
}

func (a *shutdownAgentStub) Runtime(context.Context) (localapi.RuntimeInfo, error) {
	return a.info, a.identityError
}
func (a *shutdownAgentStub) Status(context.Context) (localapi.Status, error) {
	return localapi.Status{State: a.state}, nil
}
func (a *shutdownAgentStub) PauseForMaintenance(context.Context) (localapi.OperationResult, error) {
	*a.events = append(*a.events, "pause")
	return a.paused, a.pauseError
}
func (a *shutdownAgentStub) StopForUpgrade(_ context.Context, instance string, generation int, revision string) (localapi.OperationResult, error) {
	if instance != a.info.Instance || generation != 249 || revision != strings.Repeat("b", 40) {
		return localapi.OperationResult{}, errors.New("unbound stop")
	}
	*a.events = append(*a.events, "stop")
	if a.guard {
		a.guard = false
		return localapi.OperationResult{Status: "REJECTED", Code: "guard_exit_pending"}, nil
	}
	return a.stopped, a.stopError
}

func TestSetupShutdownFencesAndConfirmsRunningAgent(t *testing.T) {
	events := []string{}
	a := &shutdownAgentStub{info: localapi.RuntimeInfo{Protocol: 1, Generation: 248, Build: strings.Repeat("a", 40), Instance: strings.Repeat("c", 32)}, state: "CONNECTED",
		paused: localapi.OperationResult{Status: "COMPLETED", Code: "disconnected"}, stopped: localapi.OperationResult{Status: "COMPLETED", Code: "upgrade_stop_requested"}, guard: true, events: &events}
	lease := &shutdownLease{}
	probes := 0
	result, err := shutdownSetup(context.Background(), 249, strings.Repeat("b", 40), setupShutdownOps{
		open: func() (setupAgent, error) { return a, nil },
		idle: func() (io.Closer, error) {
			probes++
			if probes < 3 {
				return nil, agentinstance.ErrAlreadyRunning
			}
			events = append(events, "idle")
			return lease, nil
		},
		quit: func(context.Context) error { events = append(events, "UI"); return nil },
		wait: func(context.Context) error { events = append(events, "wait"); return nil },
	})
	if err != nil || result != lease || lease.closed {
		t.Fatal("agent lease must survive publication", result, err)
	}
	if !reflect.DeepEqual(events, []string{"UI", "pause", "stop", "wait", "stop", "wait", "idle"}) {
		t.Fatal(events)
	}
	result.Close()
}

func TestSetupShutdownDoesNotInterruptUnknownNewerOrBusyAgent(t *testing.T) {
	for _, scenario := range []string{"unavailable", "newer", "different_build", "pairing", "cancelled"} {
		t.Run(scenario, func(t *testing.T) {
			events := []string{}
			a := &shutdownAgentStub{info: localapi.RuntimeInfo{Protocol: 1, Generation: 248, Build: strings.Repeat("a", 40)}, state: "CONNECTED", events: &events}
			ctx := context.Background()
			switch scenario {
			case "unavailable":
				a.identityError = localapi.ErrTransport
			case "newer":
				a.info.Generation = 250
			case "different_build":
				a.info.Generation = 249
			case "pairing":
				a.state = "PAIRING"
			case "cancelled":
				c, cancel := context.WithCancel(ctx)
				cancel()
				ctx = c
			}
			_, err := shutdownSetup(ctx, 249, strings.Repeat("b", 40), setupShutdownOps{
				open: func() (setupAgent, error) { return a, nil }, idle: func() (io.Closer, error) { return nil, agentinstance.ErrAlreadyRunning },
				quit: func(context.Context) error { events = append(events, "UI"); return nil },
			})
			if !errors.Is(err, ErrShutdown) || len(events) != 0 || ShutdownStage(err) == "" {
				t.Fatal("preflight interrupted product", events, err)
			}
		})
	}
}

func TestSetupShutdownRejectsAmbiguousDisconnectAndExit(t *testing.T) {
	for _, stage := range []string{"UI exit", "disconnect", "agent exit request", "agent exit"} {
		t.Run(stage, func(t *testing.T) {
			events := []string{}
			a := &shutdownAgentStub{info: localapi.RuntimeInfo{Protocol: 1, Generation: 248}, state: "LOCAL", events: &events,
				paused: localapi.OperationResult{Status: "REJECTED", Code: "no_active_session"}, stopped: localapi.OperationResult{Status: "COMPLETED", Code: "upgrade_stop_requested"}}
			if stage == "disconnect" {
				a.pauseError = localapi.ErrTransport
			}
			if stage == "agent exit request" {
				a.stopError = localapi.ErrTransport
			}
			_, err := shutdownSetup(context.Background(), 249, strings.Repeat("b", 40), setupShutdownOps{
				open: func() (setupAgent, error) { return a, nil }, idle: func() (io.Closer, error) { return nil, agentinstance.ErrAlreadyRunning },
				quit: func(context.Context) error {
					if stage == "UI exit" {
						return context.DeadlineExceeded
					}
					return nil
				},
				wait: func(context.Context) error { return context.DeadlineExceeded },
			})
			if !errors.Is(err, ErrShutdown) || ShutdownStage(err) != stage {
				t.Fatal("unknown shutdown accepted", err)
			}
		})
	}
}

func TestSetupShutdownMissingAgentNeedsNoAPIAndReleasesFailedUILease(t *testing.T) {
	for _, failUI := range []bool{false, true} {
		lease := &shutdownLease{}
		result, err := shutdownSetup(context.Background(), 249, strings.Repeat("b", 40), setupShutdownOps{
			idle: func() (io.Closer, error) { return lease, nil },
			quit: func(context.Context) error {
				if failUI {
					return context.DeadlineExceeded
				}
				return nil
			},
		})
		if failUI {
			if result != nil || err == nil || !lease.closed {
				t.Fatal(result, err, lease)
			}
		} else {
			if result != lease || err != nil || lease.closed {
				t.Fatal(result, err, lease)
			}
			result.Close()
		}
	}
}
