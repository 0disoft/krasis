package residentagent

import (
	"context"
	"errors"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

func TestRunOwnsLifecycleAndClosesResourcesInBoundedOrder(t *testing.T) {
	root := t.TempDir()
	if _, err := peercredentials.ProtectSecurityDirectory(root); err != nil {
		t.Fatal(err)
	}
	identityStore, err := productidentity.New(filepath.Join(root, "identity"))
	if err != nil {
		t.Fatal(err)
	}
	identity, _, err := identityStore.OpenOrCreate(time.Now().UTC())
	if err != nil {
		t.Fatal(err)
	}
	monitor := newRunMonitor()
	instance := &runLease{}
	server := &runServer{started: make(chan struct{}), stopped: make(chan struct{})}
	dependencies := runDependencies{
		acquire: func() (instanceLease, error) { return instance, nil },
		bootstrap: func(string, time.Time) (productidentity.Paths, *productidentity.Identity, bool, error) {
			return productidentity.Paths{
				Control: filepath.Join(root, "control"),
				Trust:   filepath.Join(root, "trust"),
			}, identity, false, nil
		},
		startMonitor: func() (agentlifecycle.Monitor, error) { return monitor, nil },
		listen:       func(string, localapi.Operations) (apiServer, error) { return server, nil },
		now:          time.Now,
	}
	done := make(chan error, 1)
	go func() { done <- run(context.Background(), root, dependencies) }()
	select {
	case <-server.started:
	case err := <-done:
		t.Fatalf("resident agent stopped before the API server started: %v", err)
	case <-time.After(time.Second):
		t.Fatal("resident agent did not start the API server")
	}
	monitor.events <- agentlifecycle.EventSessionLock
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("resident agent did not stop after lifecycle event")
	}
	if !instance.closed || instance.closeCalls != 1 || !server.closed || !monitor.closed {
		t.Fatalf("closed instance=%t server=%t monitor=%t", instance.closed, server.closed, monitor.closed)
	}
}

type runLease struct {
	closed     bool
	closeCalls int
	closeErr   error
}

func (lease *runLease) Close() error {
	lease.closed = true
	lease.closeCalls++
	return lease.closeErr
}

func TestRunStartupFailureReleasesOwnershipOnceAndPreservesCleanupFailure(t *testing.T) {
	for _, stage := range []string{"acquire", "recovery", "bootstrap"} {
		t.Run(stage, func(t *testing.T) {
			failure := errors.New("startup failed")
			cleanupFailure := errors.New("ownership release failed")
			lease := &runLease{closeErr: cleanupFailure}
			dependencies := runDependencies{
				acquire: func() (instanceLease, error) {
					if stage == "acquire" {
						return nil, failure
					}
					return lease, nil
				},
				checkStart: func() error {
					if stage == "recovery" {
						return failure
					}
					return nil
				},
				bootstrap: func(string, time.Time) (productidentity.Paths, *productidentity.Identity, bool, error) {
					if stage != "bootstrap" {
						t.Fatal("bootstrapped after rejected startup")
					}
					return productidentity.Paths{}, nil, false, failure
				},
				startMonitor: func() (agentlifecycle.Monitor, error) {
					t.Fatal("started monitor after startup failure")
					return nil, nil
				},
				listen: func(string, localapi.Operations) (apiServer, error) {
					t.Fatal("started server after startup failure")
					return nil, nil
				},
				now: time.Now,
			}
			err := run(context.Background(), t.TempDir(), dependencies)
			if !errors.Is(err, failure) {
				t.Fatal("lost startup failure", err)
			}
			if stage == "acquire" {
				if lease.closeCalls != 0 || errors.Is(err, cleanupFailure) {
					t.Fatal("released ownership that was not acquired", err, lease.closeCalls)
				}
			} else if lease.closeCalls != 1 || !errors.Is(err, cleanupFailure) {
				t.Fatal("ownership must be released once without losing cleanup failure", err, lease.closeCalls)
			}
		})
	}
}

type runServer struct {
	started chan struct{}
	stopped chan struct{}
	once    sync.Once
	closed  bool
}

func (server *runServer) Run(ctx context.Context) error {
	close(server.started)
	<-ctx.Done()
	return nil
}
func (server *runServer) Close(context.Context) error {
	server.once.Do(func() { server.closed = true; close(server.stopped) })
	return nil
}

type runMonitor struct {
	events chan agentlifecycle.Event
	done   chan struct{}
	once   sync.Once
	closed bool
}

func newRunMonitor() *runMonitor {
	return &runMonitor{events: make(chan agentlifecycle.Event, 1), done: make(chan struct{})}
}
func (monitor *runMonitor) Events() <-chan agentlifecycle.Event { return monitor.events }
func (monitor *runMonitor) Done() <-chan struct{}               { return monitor.done }
func (monitor *runMonitor) Close() error {
	monitor.once.Do(func() { monitor.closed = true; close(monitor.done) })
	return nil
}
