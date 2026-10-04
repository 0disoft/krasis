//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"testing"

	"golang.org/x/sys/windows"
)

func TestConsolePreparationRejectsBeforeLaunchingWithoutInput(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, ctx := range []context.Context{nil, ctx} {
		called := false
		prepared, err := prepareConsole(ctx, func(func(windows.Handle, uint32) error) (*ConsoleWorker, error) {
			called = true
			return nil, ErrAuthority
		})
		if prepared != nil || err != ErrAuthority || called {
			t.Fatal("cancelled preparation launched a child")
		}
	}
	if prepared, err := prepareAgentConsole(context.Background(), nil, [sha256.Size]byte{}, nil); prepared != nil || err != ErrAuthority {
		t.Fatal("unauthenticated preparation accepted")
	}
}

func TestPreparedConsoleClosesOnlyOwnedResourcesWithoutInput(t *testing.T) {
	server, client := nativePair(t)
	job, err := makeWorkerJob()
	if err != nil {
		t.Fatal("temporary job failed")
	}
	p := &preparedConsole{worker: &ConsoleWorker{job: job, done: make(chan struct{})}, control: server}
	if p.close() != nil || p.close() != nil || !p.worker.exited() {
		t.Fatal("preparation cleanup failed")
	}
	if _, err := client.Write([]byte{1}); err == nil {
		t.Fatal("closed control pipe remained writable")
	}
	if (*preparedConsole)(nil).close() != nil {
		t.Fatal("nil preparation cleanup failed")
	}
}
