package labcli

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/control"
)

func TestAgentSessionProcessOwnerCancelsOnLifecycleEvent(t *testing.T) {
	monitor := newProcessOwnerMonitor()
	owner, err := newAgentSessionProcessOwner(context.Background(), func() (agentlifecycle.Monitor, error) { return monitor, nil })
	if err != nil {
		t.Fatal(err)
	}
	monitor.events <- agentlifecycle.EventSessionLock
	select {
	case <-owner.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("lifecycle event did not cancel process owner")
	}
	if owner.Event() != agentlifecycle.EventSessionLock {
		t.Fatalf("owner event = %q", owner.Event())
	}
	owner.Close()
}

func TestAgentSessionProcessOwnerCancelsWhenGuardStops(t *testing.T) {
	monitor := newProcessOwnerMonitor()
	owner, err := newAgentSessionProcessOwner(context.Background(), func() (agentlifecycle.Monitor, error) { return monitor, nil })
	if err != nil {
		t.Fatal(err)
	}
	guard := newProcessOwnerGuard()
	if err := owner.SetCrashGuard(guard); err != nil {
		t.Fatal(err)
	}
	guard.stop()
	select {
	case <-owner.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("guard exit did not cancel process owner")
	}
	if !errors.Is(context.Cause(owner.Context()), errCrashGuardStopped) {
		t.Fatalf("owner cause = %v", context.Cause(owner.Context()))
	}
	owner.Close()
}

func TestAgentSessionProcessOwnerClosesSourceBeforeGuardOnce(t *testing.T) {
	monitor := newProcessOwnerMonitor()
	owner, err := newAgentSessionProcessOwner(context.Background(), func() (agentlifecycle.Monitor, error) { return monitor, nil })
	if err != nil {
		t.Fatal(err)
	}
	order := make([]string, 0, 2)
	mu := sync.Mutex{}
	source := &processOwnerSource{close: func() {
		mu.Lock()
		order = append(order, "source")
		mu.Unlock()
	}}
	guard := newProcessOwnerGuard()
	guard.close = func() {
		mu.Lock()
		order = append(order, "guard")
		mu.Unlock()
	}
	if err := owner.SetSource(source); err != nil {
		t.Fatal(err)
	}
	if err := owner.SetCrashGuard(guard); err != nil {
		t.Fatal(err)
	}
	owner.Close()
	owner.Close()
	if source.closeCalls != 1 || guard.closeCalls != 1 || len(order) != 2 || order[0] != "source" || order[1] != "guard" {
		t.Fatalf("close calls source=%d guard=%d order=%v", source.closeCalls, guard.closeCalls, order)
	}
}

type processOwnerMonitor struct {
	events chan agentlifecycle.Event
	done   chan struct{}
	once   sync.Once
}

func newProcessOwnerMonitor() *processOwnerMonitor {
	return &processOwnerMonitor{events: make(chan agentlifecycle.Event, 1), done: make(chan struct{})}
}
func (monitor *processOwnerMonitor) Events() <-chan agentlifecycle.Event { return monitor.events }
func (monitor *processOwnerMonitor) Done() <-chan struct{}               { return monitor.done }
func (monitor *processOwnerMonitor) Close() error {
	monitor.once.Do(func() { close(monitor.done) })
	return nil
}

type processOwnerSource struct {
	close      func()
	closeCalls int
}

func (*processOwnerSource) Next(context.Context) (agent.Connection, error) {
	return nil, errors.New("not used")
}
func (source *processOwnerSource) Close() error {
	source.closeCalls++
	if source.close != nil {
		source.close()
	}
	return nil
}

type processOwnerGuard struct {
	done       chan struct{}
	once       sync.Once
	close      func()
	closeCalls int
}

func newProcessOwnerGuard() *processOwnerGuard {
	return &processOwnerGuard{done: make(chan struct{})}
}
func (*processOwnerGuard) PreparePress(control.PressedToken) error { return nil }
func (*processOwnerGuard) Released(control.PressedToken) error     { return nil }
func (guard *processOwnerGuard) Done() <-chan struct{}             { return guard.done }
func (guard *processOwnerGuard) Close() error {
	guard.closeCalls++
	if guard.close != nil {
		guard.close()
	}
	guard.stop()
	return nil
}
func (guard *processOwnerGuard) stop() { guard.once.Do(func() { close(guard.done) }) }
