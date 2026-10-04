//go:build windows

package inputworker

import (
	"context"
	"crypto/sha256"
	"testing"
	"time"

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

func TestServicePreparationRejectsPortableAndMissingInstallationWithoutInput(t *testing.T) {
	for _, installation := range []*Installation{nil, {}} {
		prepared, err := prepareServiceConsole(context.Background(), installation, [sha256.Size]byte{})
		if prepared != nil || err != ErrAuthority {
			t.Fatal("uninstalled service preparation accepted")
		}
	}
}

func TestPreparedConsoleFreshnessRejectsExpiredChangedAndDeadConsole(t *testing.T) {
	if !preparedConsoleFresh(time.Millisecond, 1, 1, true) {
		t.Fatal("fresh matching console rejected")
	}
	for _, test := range []struct {
		age              time.Duration
		session, current uint32
		alive            bool
	}{{-1, 1, 1, true}, {preparedConsoleWindow, 1, 1, true}, {time.Millisecond, 1, 2, true}, {time.Millisecond, 0, 0, true}, {time.Millisecond, 0xffffffff, 0xffffffff, true}, {time.Millisecond, 1, 1, false}} {
		if preparedConsoleFresh(test.age, test.session, test.current, test.alive) {
			t.Fatal("stale console preparation reused")
		}
	}
}

func TestPreparedConsoleExpiryCannotCloseClaimedWorkerWithoutInput(t *testing.T) {
	for _, claimed := range []bool{false, true} {
		server, client := nativePair(t)
		job, err := makeWorkerJob()
		if err != nil {
			t.Fatal(err)
		}
		p := &preparedConsole{worker: &ConsoleWorker{job: job, done: make(chan struct{})}, control: server}
		if claimed {
			p.warmState.Store(1)
		}
		if p.retireUnbound() != nil {
			t.Fatal("expiry cleanup failed")
		}
		if p.worker.exited() == claimed {
			t.Fatal("expiry closed claimed child or retained unbound child")
		}
		if p.claim(context.Background()) {
			t.Fatal("claimed/expired child was claimed again")
		}
		if p.close() != nil {
			t.Fatal("final preparation cleanup failed")
		}
		client.Close()
	}
}

func TestPreparedConsoleExpiryRetainsCleanupFailureWithoutInput(t *testing.T) {
	// InvalidHandle (-1) is also the current-process pseudo handle. Use an
	// impossible unaligned positive kernel handle for a real close failure.
	p := &preparedConsole{worker: &ConsoleWorker{job: windows.Handle(0x7ffffffe), done: make(chan struct{})}}
	if p.retireUnbound() == nil || p.close() == nil {
		t.Fatal("asynchronous preparation cleanup failure lost")
	}
	if p.claim(context.Background()) {
		t.Fatal("failed cleanup preparation was reused")
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
