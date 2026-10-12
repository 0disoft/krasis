package residentagent

import (
	"context"
	"sync/atomic"
	"testing"
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
