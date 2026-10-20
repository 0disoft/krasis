//go:build windows

package inputworker

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestNativeAuthorityQueryKeepsPipeAndProcessUntilLateCompletion(t *testing.T) {
	_, pipe := nativePair(t)
	process, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, windows.GetCurrentProcessId())
	if err != nil {
		t.Fatal(err)
	}
	channel := clientChannel(pipe, authorityService)
	var closed atomic.Bool
	channel.authority.closeHandles = func() error {
		closed.Store(true)
		return windows.CloseHandle(process)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	checked := make(chan error, 1)
	channel.authority.checkInputObserved = func(context.Context) error {
		close(entered)
		<-release
		var pid, exit uint32
		err := windows.GetNamedPipeServerProcessId(pipe.handle, &pid)
		if err == nil {
			err = windows.GetExitCodeProcess(process, &exit)
		}
		if err == nil && (pid != windows.GetCurrentProcessId() || closed.Load()) {
			err = ErrAuthority
		}
		checked <- err
		return err
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- channel.authorizeInput(ctx, true) }()
	<-entered
	channel.queryMu.Lock()
	query := channel.query
	channel.queryMu.Unlock()
	cancel()
	err = <-done
	if !errors.Is(err, context.Canceled) || closed.Load() || channel.Close() != ErrAuthority {
		t.Error("native query cancellation discarded handles or claimed confirmed cleanup", err)
	}
	close(release)
	select {
	case <-query.done:
	case <-time.After(time.Second):
		t.Fatal("native query did not finish retirement")
	}
	if err := <-checked; err != nil {
		t.Fatal("late native query lost its exact pipe/process handles", err)
	}
	if !closed.Load() || channel.Close() != nil || channel.authority.check(false) == nil {
		t.Fatal("native handles leaked or retired proof regained authority")
	}
}
