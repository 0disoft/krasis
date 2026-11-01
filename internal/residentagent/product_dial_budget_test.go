package residentagent

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/truststore"
)

type budgetDialConnection struct {
	agent.Connection
	authenticate func() (transport.AuthenticatedPeerContext, error)
	closes       int
	closeErr     error
}

func (c *budgetDialConnection) AuthenticatedPeer() (transport.AuthenticatedPeerContext, error) {
	return c.authenticate()
}
func (c *budgetDialConnection) Close() error { c.closes++; return c.closeErr }

type productDialFixture struct {
	source                         *boundedProductDial
	options                        productDialOptions
	connection                     *budgetDialConnection
	now, start                     time.Time
	loads, dials, discovers, saves int
	at                             func(string, context.Context)
}

func newProductDialFixture() *productDialFixture {
	// Real context timers remain in the future; only injected time advances.
	// This models a delayed timer callback without wall-clock sleeps or sockets.
	f := &productDialFixture{start: time.Now().Add(time.Hour), at: func(string, context.Context) {}}
	f.now = f.start
	profile := truststore.ConnectionProfile{Fingerprint: strings.Repeat("a", 64),
		ConnectionSettings: truststore.ConnectionSettings{Address: "127.0.0.1:4242", Edge: "RIGHT"}}
	f.connection = &budgetDialConnection{authenticate: func() (transport.AuthenticatedPeerContext, error) {
		f.at("authenticate", nil)
		return transport.AuthenticatedPeerContext{CertificateSHA256: profile.Fingerprint, Role: transport.AuthenticatedPeerListener}, nil
	}}
	f.source = &boundedProductDial{fingerprint: profile.Fingerprint,
		load: func() (truststore.ConnectionProfile, error) {
			f.loads++
			stage := "load"
			if f.loads > 1 {
				stage = "reload"
			}
			f.at(stage, nil)
			return profile, nil
		},
		discover: func(ctx context.Context) ([]string, error) {
			f.discovers++
			f.at("discover", ctx)
			return []string{"127.0.0.2:4242"}, nil
		},
		save: func(truststore.ConnectionProfile, string) error {
			f.saves++
			f.at("save", nil)
			return nil
		},
		observe: func(code string) { f.at(code, nil) },
	}
	f.options = productDialOptions{now: func() time.Time { return f.now },
		dial: func(ctx context.Context, address string, timeout time.Duration) (agent.Connection, error) {
			f.dials++
			if address == profile.Address {
				f.at("primary", ctx)
				return nil, errors.New("old address unreachable")
			}
			f.at("candidate", ctx)
			return f.connection, nil
		},
	}
	return f
}

func TestProductDialTotalBudgetAtIOBoundaries(t *testing.T) {
	for _, stage := range []string{"load", "discover", "authenticate", "reload", "save", "address_discovery_confirmed"} {
		for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
			t.Run(stage+"/"+delta.String(), func(t *testing.T) {
				f := newProductDialFixture()
				f.at = func(at string, _ context.Context) {
					if at == stage {
						f.now = f.start.Add(12*time.Second + delta)
					}
				}
				connection, err := f.source.next(context.Background(), f.options)
				if delta < 0 {
					if err != nil || connection != f.connection || f.saves != 1 || f.connection.closes != 0 {
						t.Fatalf("in-budget result: connection=%v err=%v saves=%d closes=%d", connection, err, f.saves, f.connection.closes)
					}
					return
				}
				wantClose, wantSave := 0, 0
				if stage != "load" && stage != "discover" {
					wantClose = 1
				}
				if stage == "save" || stage == "address_discovery_confirmed" {
					wantSave = 1
				}
				if connection != nil || !errors.Is(err, context.DeadlineExceeded) || f.connection.closes != wantClose || f.saves != wantSave {
					t.Fatalf("late result: connection=%v err=%v closes=%d saves=%d", connection, err, f.connection.closes, f.saves)
				}
				if stage == "load" && f.dials != 0 || stage == "discover" && f.dials != 1 {
					t.Fatal("expired I/O admitted another dial", f.dials)
				}
			})
		}
	}
}

func TestProductDialAttemptDeadlineBoundaries(t *testing.T) {
	for _, candidate := range []bool{false, true} {
		for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
			t.Run(fmt.Sprintf("candidate=%t/%s", candidate, delta), func(t *testing.T) {
				f := newProductDialFixture()
				if !candidate {
					f.source.discover = nil
				}
				f.options.dial = func(ctx context.Context, _ string, timeout time.Duration) (agent.Connection, error) {
					f.dials++
					if candidate && f.dials == 1 {
						return nil, errors.New("unreachable")
					}
					want := 5 * time.Second
					if candidate {
						want = time.Second
					}
					deadline, ok := ctx.Deadline()
					if !ok || timeout != want || deadline.Sub(f.now) != want {
						t.Fatal("incorrect attempt budget", timeout, deadline.Sub(f.now))
					}
					f.now = deadline.Add(delta)
					return f.connection, nil
				}
				connection, err := f.source.next(context.Background(), f.options)
				if delta < 0 {
					if err != nil || connection != f.connection || f.connection.closes != 0 {
						t.Fatal("in-budget dial rejected", err)
					}
				} else if err == nil || connection != nil || f.connection.closes != 1 || f.saves != 0 {
					t.Fatalf("late dial accepted/leaked: connection=%v err=%v closes=%d saves=%d", connection, err, f.connection.closes, f.saves)
				}
			})
		}
	}
}

func TestProductDialCancellationStopsSubsequentIO(t *testing.T) {
	for _, extraSignal := range []bool{false, true} {
		for _, stage := range []string{"before", "load", "primary", "discover", "candidate", "authenticate", "reload", "save"} {
			t.Run(stage+map[bool]string{true: "/abort", false: "/caller"}[extraSignal], func(t *testing.T) {
				f := newProductDialFixture()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				caller := ctx
				if extraSignal {
					f.options.signal = ctx
					caller = context.Background()
				}
				f.at = func(at string, _ context.Context) {
					if at == stage {
						cancel()
					}
				}
				if stage == "before" {
					cancel()
				}
				connection, err := f.source.next(caller, f.options)
				if connection != nil || !errors.Is(err, context.Canceled) {
					t.Fatal("cancellation admitted connection", err)
				}
				wantSave, wantClose := 0, 0
				if stage == "save" {
					wantSave = 1
				}
				if stage == "candidate" || stage == "authenticate" || stage == "reload" || stage == "save" {
					wantClose = 1
				}
				if f.saves != wantSave || f.connection.closes != wantClose {
					t.Fatal("canceled I/O leaked/saved", f.saves, f.connection.closes)
				}
				if stage == "before" && f.loads != 0 || stage == "load" && f.dials != 0 || stage == "primary" && f.discovers != 0 || stage == "discover" && f.dials != 1 {
					t.Fatal("I/O started after cancellation", f.loads, f.dials, f.discovers)
				}
			})
		}
	}
}

func TestProductDialAbortInterruptsOutstandingDial(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		f := newProductDialFixture()
		signal, abort := context.WithCancel(context.Background())
		defer abort()
		f.options.signal = signal
		f.options.dial = func(ctx context.Context, _ string, _ time.Duration) (agent.Connection, error) {
			abort()
			<-ctx.Done()
			return f.connection, nil // Native success races cancellation.
		}
		connection, err := f.source.next(context.Background(), f.options)
		if connection != nil || !errors.Is(err, context.Canceled) || f.connection.closes != 1 || f.discovers != 0 {
			t.Fatal("abort did not interrupt/retire outstanding dial", connection, err)
		}
	})
}

func TestProductDialDiscoveryCooldownBoundary(t *testing.T) {
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		t.Run(delta.String(), func(t *testing.T) {
			f := newProductDialFixture()
			f.source.lastDiscovery = f.now.Add(-15*time.Second - delta)
			connection, err := f.source.next(context.Background(), f.options)
			if delta < 0 {
				if err == nil || connection != nil || f.discovers != 0 {
					t.Fatal("cooldown bypassed", err)
				}
			} else if err != nil || connection != f.connection || f.discovers != 1 {
				t.Fatal("eligible discovery rejected", err)
			}
		})
	}
}

func TestProductDialEarlierCallerDeadlineWins(t *testing.T) {
	for _, delta := range []time.Duration{-time.Nanosecond, 0, time.Nanosecond} {
		t.Run(delta.String(), func(t *testing.T) {
			f := newProductDialFixture()
			deadline := f.start.Add(2 * time.Second)
			ctx, cancel := context.WithDeadline(context.Background(), deadline)
			defer cancel()
			f.options.dial = func(run context.Context, _ string, _ time.Duration) (agent.Connection, error) {
				if got, _ := run.Deadline(); got != deadline {
					t.Fatal("attempt extended caller deadline", got)
				}
				f.now = deadline.Add(delta)
				return f.connection, nil
			}
			connection, err := f.source.next(ctx, f.options)
			if delta < 0 {
				if connection != f.connection || err != nil {
					t.Fatal("in-budget connection rejected", err)
				}
			} else if connection != nil || !errors.Is(err, context.DeadlineExceeded) || f.connection.closes != 1 || f.discovers != 0 {
				t.Fatal("caller budget was extended", connection, err, f.connection.closes, f.discovers)
			}
		})
	}
}

func TestProductDialFailureNeverPublishesCandidate(t *testing.T) {
	for _, stage := range []string{"dial", "empty_dial", "authenticate", "save", "close"} {
		t.Run(stage, func(t *testing.T) {
			f := newProductDialFixture()
			failure := errors.New("native failure")
			switch stage {
			case "dial", "empty_dial":
				f.source.discover = nil
				f.options.dial = func(context.Context, string, time.Duration) (agent.Connection, error) {
					if stage == "empty_dial" {
						return nil, nil
					}
					return f.connection, failure
				}
			case "authenticate":
				f.connection.authenticate = func() (transport.AuthenticatedPeerContext, error) {
					return transport.AuthenticatedPeerContext{}, failure
				}
			case "save":
				f.source.save = func(truststore.ConnectionProfile, string) error { return failure }
			case "close":
				f.connection.closeErr = failure
				f.at = func(at string, _ context.Context) {
					if at == "reload" {
						f.now = f.start.Add(12 * time.Second)
					}
				}
			}
			connection, err := f.source.next(context.Background(), f.options)
			wantClose := 1
			if stage == "empty_dial" {
				wantClose = 0
			}
			if connection != nil || err == nil || f.connection.closes != wantClose {
				t.Fatal("failed candidate leaked", connection, err, f.connection.closes)
			}
			if stage == "close" && (!errors.Is(err, failure) || !errors.Is(err, context.DeadlineExceeded)) {
				t.Fatal("cleanup lost native cause", err)
			}
		})
	}
}
