package productpanel

import (
	"context"
	"testing"
	"time"
)

func TestLifecycleGateCancellationDoesNotConsumeFutureLock(t *testing.T) {
	var gate lifecycleGate
	gate.Lock()
	ctx, cancel := context.WithCancel(context.Background())
	waited := make(chan bool, 1)
	go func() { waited <- gate.LockContext(ctx) }()
	cancel()
	select {
	case acquired := <-waited:
		if acquired {
			t.Fatal("cancelled waiter acquired held gate")
		}
	case <-time.After(time.Second):
		t.Fatal("cancelled waiter did not exit")
	}
	if gate.TryLock() {
		t.Fatal("held gate became available")
	}
	gate.Unlock()
	if !gate.TryLock() {
		t.Fatal("cancelled waiter consumed released gate")
	}
	gate.Unlock()
	ctx, cancel = context.WithCancel(context.Background())
	cancel()
	if gate.LockContext(ctx) || !gate.TryLock() {
		t.Fatal("pre-cancelled waiter consumed available gate")
	}
	gate.Unlock()
}

func TestReceivedUpdateWaitsForBusyLifecycleInsteadOfDroppingPoll(t *testing.T) {
	c := newReceiverClient()
	b := NewDesktopLifecycle(context.Background(), nil, nil, nil)
	receiver := NewWorkspaceUpdateReceiver(b.ctx, func() (UpdateSettingsClient, error) { return c, nil }, func(string, string, string, string) error { return nil })
	b.gate.Lock()
	result := make(chan bool, 1)
	go func() { result <- b.ApplyReceivedUpdate(receiver) }()
	select {
	case <-result:
		b.gate.Unlock()
		t.Fatal("update poll was dropped while lifecycle was busy")
	case <-time.After(30 * time.Millisecond):
	}
	b.gate.Unlock()
	select {
	case applied := <-result:
		if !applied || c.applied != 1 || !b.stopped.Load() {
			t.Fatal("queued update did not hand off once")
		}
	case <-time.After(time.Second):
		t.Fatal("released lifecycle did not admit update")
	}
}

func TestReceivedUpdateShutdownCancelsWaitWithoutClaim(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	b := NewDesktopLifecycle(ctx, nil, nil, nil)
	opened := 0
	receiver := NewWorkspaceUpdateReceiver(ctx, func() (UpdateSettingsClient, error) { opened++; return newReceiverClient(), nil }, func(string, string, string, string) error { t.Fatal("shutdown launched replacement"); return nil })
	b.gate.Lock()
	result := make(chan bool, 1)
	go func() { result <- b.ApplyReceivedUpdate(receiver) }()
	cancel()
	select {
	case applied := <-result:
		if applied || opened != 0 {
			t.Fatal("shutdown still polled update")
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown blocked on lifecycle lock")
	}
	b.gate.Unlock()
}
