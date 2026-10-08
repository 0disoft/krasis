//go:build windows && desktop

package workspacehost

import (
	"context"
	"testing"
	"time"
)

func TestServiceShutdownCancelsWorkersWithoutApplicationShutdownCallback(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &WorkspaceService{ctx: ctx, cancel: cancel, done: make(chan struct{})}
	s.jobs.Add(1)
	go func() { defer s.jobs.Done(); <-ctx.Done(); close(s.done) }()
	finished := make(chan error, 1)
	go func() { finished <- s.ServiceShutdown() }()
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("service shutdown waited before cancelling its workers")
	}
}
