package residentagent

import (
	"context"
	"strings"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"

	"github.com/0disoft/krasis/internal/inputworker"
)

func TestProtectedReturnCursorRestoreJoinsPendingOpenBeforeNewLease(t *testing.T) {
	started, finished := make(chan struct{}), make(chan struct{})
	var opens atomic.Int32
	input := &privilegedLeaseInput{}
	input.cursor.root = context.Background()
	input.cursor.open = func(ctx context.Context, lease inputworker.Lease) (func() error, error) {
		opens.Add(1)
		close(started)
		<-ctx.Done()
		close(finished)
		return nil, ctx.Err()
	}
	// Exercise the same retirement gate while an authenticated open is pending.
	ctx, cancel := context.WithCancel(context.Background())
	input.cursor.cancel = cancel
	input.cursor.done = make(chan struct{})
	go func() { defer close(input.cursor.done); _, _ = input.cursor.open(ctx, inputworker.Lease{}) }()
	<-started
	input.RestoreReturnCursor()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("pending owner was not joined")
	}
	if input.cursor.cancel != nil || input.cursor.done != nil || opens.Load() != 1 {
		t.Fatal("retirement retained or reopened passive owner")
	}
}

func TestReturnCursorRetirementTimingIncludesJoinAfterProfileWindow(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input := &privilegedLeaseInput{}
		ctx, cancel := context.WithCancel(context.Background())
		input.cursor.cancel, input.cursor.done = cancel, make(chan struct{})
		go func() {
			<-ctx.Done()
			time.Sleep(150 * time.Millisecond)
			close(input.cursor.done)
		}()
		var events []string
		input.event = func(code string) {
			if !input.cursor.mu.TryLock() {
				t.Fatal("retirement observer retained cursor admission gate")
			}
			input.cursor.mu.Unlock()
			events = append(events, code)
		}
		// No handoff collector or profile context is needed for this observation.
		time.Sleep(3 * time.Minute)
		input.RestoreReturnCursor()
		if len(events) != 1 || events[0] != "installed_input_handoff_timing_cursor_retire_lt1s" {
			t.Fatal("join wait disappeared or was measured before completion", events)
		}
		input.RestoreReturnCursor()
		if len(events) != 1 {
			t.Fatal("empty retirement emitted a false worker wait")
		}
	})
}

func TestProtectedReturnCursorStaleActionCannotHideNewEntry(t *testing.T) {
	input := &privilegedLeaseInput{client: &testCursorClient{}, identity: application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, leaseExpires: time.Now().Add(time.Minute)}
	input.cursor.root = context.Background()
	input.cursor.open = func(context.Context, inputworker.Lease) (func() error, error) {
		t.Error("stale action opened presentation")
		return nil, inputworker.ErrAuthority
	}
	action := input.PrepareReturnCursor()
	if action == nil {
		t.Fatal("could not prepare snapshot from current input lease")
	}
	input.RestoreReturnCursor()
	action()
	if input.cursor.cancel != nil {
		t.Fatal("late return survived new entry")
	}
}

func TestProtectedReturnCursorSkipsBusyInputWithoutWaiting(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client := &testCursorClient{}
		identity := application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}
		input := &privilegedLeaseInput{client: client, identity: identity, leaseExpires: time.Now().Add(time.Minute)}
		input.cursor.root = context.Background()
		input.cursor.open = func(context.Context, inputworker.Lease) (func() error, error) {
			t.Error("snapshot preparation opened a presentation worker")
			return nil, inputworker.ErrAuthority
		}
		// Model an in-flight input/native owner retaining the admission gate.
		input.mu.Lock()
		released := make(chan struct{})
		go func() {
			time.Sleep(application.InputInjectionTimeout)
			input.mu.Unlock()
			close(released)
		}()
		started := time.Now()
		if action := input.PrepareReturnCursor(); action != nil || time.Since(started) != 0 || client.reads != 0 {
			t.Error("optional cursor snapshot waited behind input or performed a query", time.Since(started), client.reads)
		}
		if input.mu.TryLock() {
			input.mu.Unlock()
			t.Error("snapshot waited for or released the active input owner's gate")
		}
		<-released
		if input.client != client || input.identity != identity || client.closes.Load() != 0 {
			t.Fatal("skipping optional presentation changed the active input lease")
		}
		before := client.reads
		if action := input.PrepareReturnCursor(); action == nil || client.reads != before+1 {
			t.Fatal("idle input no longer permits the normal cursor snapshot", client.reads)
		}
	})
}

func TestReentryCancellationDoesNotReportCursorStartupFailure(t *testing.T) {
	started := make(chan struct{})
	input := &privilegedLeaseInput{client: &testCursorClient{}, identity: application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, leaseExpires: time.Now().Add(time.Minute)}
	input.cursor.root = context.Background()
	input.cursor.open = func(ctx context.Context, _ inputworker.Lease) (func() error, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}
	var cancelled atomic.Int32
	input.event = func(code string) {
		if code == "installed_return_cursor_failed" {
			t.Error("reentry was classified as startup failure")
		}
		if code == "installed_return_cursor_cancelled" {
			cancelled.Add(1)
		} else if !strings.HasPrefix(code, "installed_input_handoff_timing_") {
			t.Error("unexpected cursor event", code)
		}
	}
	action := input.PrepareReturnCursor()
	if action == nil {
		t.Fatal("missing cursor action")
	}
	action()
	<-started
	input.RestoreReturnCursor()
	if cancelled.Load() != 1 {
		t.Fatal("cancelled owner was not joined/classified")
	}
}
