//go:build windows

package inputworker

import (
	"context"
	"testing"

	"golang.org/x/sys/windows"
)

func TestLoginConsoleRejectsUntrustedLaunchWithoutCreatingProcess(t *testing.T) {
	prepare := func(windows.Handle, uint32) error {
		t.Fatal("untrusted caller reached child bootstrap")
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, request := range []struct {
		ctx          context.Context
		installation *Installation
		session      uint32
		profile      [32]byte
		prepare      func(windows.Handle, uint32) error
	}{
		{nil, nil, 1, [32]byte{1}, prepare},
		{ctx, nil, 1, [32]byte{1}, prepare},
		{context.Background(), nil, 1, [32]byte{1}, prepare},
		{context.Background(), &Installation{}, 1, [32]byte{1}, prepare},
		{context.Background(), &Installation{}, 0, [32]byte{1}, prepare},
		{context.Background(), &Installation{}, ^uint32(0), [32]byte{1}, prepare},
		{context.Background(), &Installation{}, 1, [32]byte{}, prepare},
		{context.Background(), &Installation{}, 1, [32]byte{1}, nil},
	} {
		worker, err := launchLoginConsole(request.ctx, request.installation, request.session, request.profile, request.prepare)
		if worker != nil || err != ErrAuthority {
			t.Fatal("untrusted login worker launched", err)
		}
	}
}

func TestConsoleLaunchHasOnlyTwoFixedModes(t *testing.T) {
	for _, mode := range []consoleMode{ordinaryConsole, loginConsole} {
		arguments, desktop, err := consoleLaunchMode(mode)
		if err != nil {
			t.Fatal(err)
		}
		if mode == ordinaryConsole && (arguments != " --input-worker" || desktop != `winsta0\default`) {
			t.Fatal("ordinary input mode changed")
		}
		if mode == loginConsole && (arguments != " --login-worker" || desktop != `winsta0\Winlogon`) {
			t.Fatal("login worker selected an unbound mode")
		}
	}
	for _, mode := range []consoleMode{2, 255} {
		if _, _, err := consoleLaunchMode(mode); err != ErrAuthority {
			t.Fatal("unknown desktop/mode accepted")
		}
	}
}

func TestConsoleCleanupFailureNeverSignalsRetirement(t *testing.T) {
	job, err := makeWorkerJob()
	if err != nil {
		t.Fatal(err)
	}
	worker := &ConsoleWorker{job: job, done: make(chan struct{})}
	// Simulate a lost owned handle; Close must not report Done as successful.
	if err := windows.CloseHandle(job); err != nil {
		t.Fatal(err)
	}
	if worker.Close() == nil || worker.Close() == nil {
		t.Fatal("cleanup failure discarded")
	}
	select {
	case <-worker.done:
		t.Fatal("failed retirement published Done")
	default:
	}
}
