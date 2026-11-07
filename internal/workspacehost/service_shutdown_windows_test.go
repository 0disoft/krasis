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

func TestShutdownRejectsLateCallbackWorkAndWaitsForAdmittedWork(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	s := &WorkspaceService{ctx: ctx, cancel: cancel}
	started, release := make(chan struct{}), make(chan struct{})
	if !s.startJob(func() { close(started); <-release }) {
		t.Fatal("live worker was not admitted")
	}
	<-started
	finished := make(chan error, 1)
	go func() { finished <- s.ServiceShutdown() }()
	<-ctx.Done()
	if s.startJob(func() { t.Error("late worker ran") }) {
		t.Fatal("shutdown admitted a late callback")
	}
	select {
	case <-finished:
		t.Fatal("shutdown did not wait for admitted work")
	default:
	}
	close(release)
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("shutdown did not finish")
	}
}
