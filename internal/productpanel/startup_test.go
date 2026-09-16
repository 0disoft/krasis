package productpanel

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
	"io"
	"testing"
)

type runtimeStub struct {
	state        string
	generation   int
	legacy, busy bool
	stops        int
}

func (r *runtimeStub) Status(context.Context) (localapi.Status, error) {
	if r.state != "" {
		return localapi.Status{State: r.state}, nil
	}
	return localapi.Status{State: "LOCAL"}, nil
}

func TestStartupRecoversFaultedProcessWithoutDowngradeOrRestartLoop(t *testing.T) {
	for _, test := range []struct {
		name                         string
		generation                   int
		occupied, badBundle, refault bool
		wantStarts, wantStops        int
		wantErr                      bool
	}{
		{"same generation", 2, false, false, false, 1, 1, false},
		{"older generation", 1, false, false, false, 1, 1, false},
		{"newer generation", 3, false, false, false, 0, 0, true},
		{"owner still alive", 2, true, false, false, 0, 1, true},
		{"bad bundle", 2, false, true, false, 0, 0, true},
		{"replacement faults", 2, false, false, true, 1, 1, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &runtimeStub{generation: test.generation, state: "FAULTED"}
			starts, pauses, probes := 0, 0, 0
			s := Startup{Generation: 2, Lock: func() (io.Closer, error) { return closerStub{}, nil }, Open: func() (RuntimeClient, error) { return r, nil },
				Verify: func() error {
					if test.badBundle {
						return errors.New("bad bundle")
					}
					return nil
				},
				Available: func() (bool, error) { probes++; return !test.occupied, nil },
				Start: func() error {
					if probes == 0 || r.stops != 1 {
						t.Fatal("start before stop and ownership verification")
					}
					starts++
					r.generation = 2
					if !test.refault {
						r.state = "LOCAL"
					}
					return nil
				},
				Pause: func(context.Context) error {
					pauses++
					if pauses > 4 {
						return context.DeadlineExceeded
					}
					return nil
				},
			}
			err := s.Ensure(context.Background())
			if (err != nil) != test.wantErr || starts != test.wantStarts || r.stops != test.wantStops {
				t.Fatalf("err=%v starts=%d stops=%d", err, starts, r.stops)
			}
		})
	}
}

func TestFaultRecoveryActionIsExplicit(t *testing.T) {
	if RecoveryAction("FAULTED", "disconnect") != "prepare" || DisconnectLabel("FAULTED") != "에이전트 재시작" {
		t.Fatal("fault recovery unavailable")
	}
	for _, state := range []string{"LOCAL", "CONNECTED", "RECONNECTING", "FAULTED"} {
		if RecoveryAction(state, "refresh") != "refresh" {
			t.Fatal("poll restarted agent")
		}
		if state != "FAULTED" && RecoveryAction(state, "disconnect") != "disconnect" {
			t.Fatal("normal disconnect changed")
		}
	}
}
func (r *runtimeStub) Runtime(context.Context) (localapi.RuntimeInfo, error) {
	if r.legacy {
		return localapi.RuntimeInfo{}, localapi.ErrNotFound
	}
	return localapi.RuntimeInfo{Protocol: 1, Generation: r.generation, Instance: "instance"}, nil
}
func (r *runtimeStub) StopForUpgrade(context.Context, string) (localapi.OperationResult, error) {
	r.stops++
	if r.busy {
		return localapi.OperationResult{Status: "REJECTED"}, nil
	}
	return localapi.OperationResult{Status: "COMPLETED", Code: "upgrade_stop_requested"}, nil
}

type closerStub struct{}

func (closerStub) Close() error { return nil }

func TestStartupReusesStartsAndGuardsUpgrade(t *testing.T) {
	for _, test := range []struct {
		name                              string
		missing, legacy, busy, badBundle  bool
		generation, wantStarts, wantStops int
		wantErr                           bool
	}{
		{"missing", true, false, false, false, 2, 1, 0, false},
		{"matching", false, false, false, false, 2, 0, 0, false},
		{"newer", false, false, false, false, 3, 0, 0, false},
		{"legacy", false, true, false, false, 0, 0, 0, false},
		{"upgrade", false, false, false, false, 1, 1, 1, false},
		{"busy", false, false, true, false, 1, 0, 1, true},
		{"tampered", true, false, false, true, 2, 0, 0, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			r := &runtimeStub{generation: test.generation, legacy: test.legacy, busy: test.busy}
			missing, started, pauses, availableCalls := test.missing, 0, 0, 0
			s := Startup{Generation: 2, Lock: func() (io.Closer, error) { return closerStub{}, nil }, Open: func() (RuntimeClient, error) {
				if missing {
					return nil, localapi.ErrUnavailable
				}
				return r, nil
			},
				Verify: func() error {
					if test.badBundle {
						return errors.New("bad hash")
					}
					return nil
				},
				Available: func() (bool, error) { availableCalls++; return test.name != "upgrade" || availableCalls > 1, nil },
				Start: func() error {
					if availableCalls == 0 || (test.name == "upgrade" && availableCalls < 2) {
						t.Fatal("start before ownership check")
					}
					started++
					missing = false
					r.generation = 2
					return nil
				},
				Pause: func(context.Context) error {
					pauses++
					if pauses > 4 {
						return context.DeadlineExceeded
					}
					return nil
				},
			}
			err := s.Ensure(context.Background())
			if (err != nil) != test.wantErr || started != test.wantStarts || r.stops != test.wantStops {
				t.Fatalf("err=%v starts=%d stops=%d", err, started, r.stops)
			}
		})
	}
}

func TestStartupWaitsForOwnershipAndNeverLoopsLaunch(t *testing.T) {
	starts := 0
	s := Startup{Generation: 1, Lock: func() (io.Closer, error) { return closerStub{}, nil }, Open: func() (RuntimeClient, error) { return nil, localapi.ErrUnavailable }, Available: func() (bool, error) { return true, nil }, Verify: func() error { return nil }, Start: func() error { starts++; return nil }, Pause: func(context.Context) error { return context.DeadlineExceeded }}
	if !errors.Is(s.Ensure(context.Background()), ErrAgentNotReady) || starts != 1 {
		t.Fatal("startup was not bounded")
	}
	starts = 0
	s.Open = func() (RuntimeClient, error) { return nil, localapi.ErrTransport }
	if !errors.Is(s.Ensure(context.Background()), ErrAgentNotReady) || starts != 1 {
		t.Fatal("stale endpoint did not allow one verified start")
	}
	starts = 0
	s.Open = func() (RuntimeClient, error) { return nil, localapi.ErrAuthentication }
	if !errors.Is(s.Ensure(context.Background()), localapi.ErrAuthentication) || starts != 0 {
		t.Fatal("authentication failure was bypassed")
	}
	s.Open = func() (RuntimeClient, error) { return nil, localapi.ErrUnavailable }
	s.Available = func() (bool, error) { return false, nil }
	s.Ensure(context.Background())
	if starts != 0 {
		t.Fatal("started over existing owner")
	}
	s.Lock = func() (io.Closer, error) { return nil, errors.New("locked") }
	if !errors.Is(s.Ensure(context.Background()), ErrStartupBusy) {
		t.Fatal("concurrent start allowed")
	}
}
