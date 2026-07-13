package agentlifecycle

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestOwnerCancelsOnFirstLifecycleEvent(t *testing.T) {
	monitor := newFakeMonitor()
	owner, err := Start(context.Background(), func() (Monitor, error) { return monitor, nil })
	if err != nil {
		t.Fatal(err)
	}
	monitor.events <- EventSuspend
	select {
	case <-owner.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("lifecycle event did not cancel the owner")
	}
	var transition *TransitionError
	if !errors.As(context.Cause(owner.Context()), &transition) || transition.Event != EventSuspend {
		t.Fatalf("owner cause = %v", context.Cause(owner.Context()))
	}
	if owner.Event() != EventSuspend {
		t.Fatalf("owner event = %q", owner.Event())
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerTreatsUnexpectedMonitorExitAsFailure(t *testing.T) {
	monitor := newFakeMonitor()
	owner, err := Start(context.Background(), func() (Monitor, error) { return monitor, nil })
	if err != nil {
		t.Fatal(err)
	}
	monitor.stop()
	select {
	case <-owner.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("monitor exit did not cancel the owner")
	}
	if !errors.Is(context.Cause(owner.Context()), ErrMonitorStopped) {
		t.Fatalf("owner cause = %v", context.Cause(owner.Context()))
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestOwnerCloseIsIdempotentAndDoesNotInventLifecycleEvent(t *testing.T) {
	monitor := newFakeMonitor()
	owner, err := Start(context.Background(), func() (Monitor, error) { return monitor, nil })
	if err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
	if monitor.closeCalls != 1 || owner.Event() != "" {
		t.Fatalf("close calls=%d event=%q", monitor.closeCalls, owner.Event())
	}
}

func TestOwnerRejectsInvalidMonitorAndCanceledParent(t *testing.T) {
	if _, err := Start(context.Background(), func() (Monitor, error) { return nil, nil }); err == nil {
		t.Fatal("Start accepted a nil monitor")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Start(ctx, func() (Monitor, error) { return newFakeMonitor(), nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("Start error = %v", err)
	}
}

type fakeMonitor struct {
	events     chan Event
	done       chan struct{}
	once       sync.Once
	closeCalls int
}

func newFakeMonitor() *fakeMonitor {
	return &fakeMonitor{events: make(chan Event, 1), done: make(chan struct{})}
}

func (monitor *fakeMonitor) Events() <-chan Event  { return monitor.events }
func (monitor *fakeMonitor) Done() <-chan struct{} { return monitor.done }
func (monitor *fakeMonitor) Close() error {
	monitor.closeCalls++
	monitor.stop()
	return nil
}
func (monitor *fakeMonitor) stop() { monitor.once.Do(func() { close(monitor.done) }) }
