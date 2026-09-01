package residentagent

import (
	"context"
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
	if !instance.closed || !server.closed || !monitor.closed {
		t.Fatalf("closed instance=%t server=%t monitor=%t", instance.closed, server.closed, monitor.closed)
	}
}

type runLease struct{ closed bool }

func (lease *runLease) Close() error { lease.closed = true; return nil }

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
