package productpanel

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestInitialAndPendingHealthReadsNeverBlockRendererOrSpawnDuplicateReaders(t *testing.T) {
	for _, initial := range []string{"", "PENDING"} {
		t.Run(initial, func(t *testing.T) {
			entered, finish := make(chan struct{}), make(chan struct{})
			var probes atomic.Int32
			b := &inputRefresh{inspect: func() string { probes.Add(1); close(entered); <-finish; return "READY" },
				run: func(context.Context) error { t.Fatal("renderer installed input"); return nil }}
			if initial != "" {
				b.setState(initial)
				b.checkedAt = time.Now().Add(-2 * time.Second)
			}
			want := initial
			if want == "" {
				want = "UNKNOWN"
			}
			if b.status() != want {
				t.Fatal("first query waited or invented ready state")
			}
			<-entered
			for range 4 {
				if b.status() != want || b.recheck() != want {
					t.Fatal("reader blocked renderer")
				}
			}
			if probes.Load() != 1 {
				t.Fatal("duplicate reader", probes.Load())
			}
			close(finish)
			b.mu.Lock()
			b.mu.Unlock()
			if b.status() != "READY" {
				t.Fatal("completed observation not published")
			}
		})
	}
}

func TestAutomaticPreparationAwaitsHealthReaderWithoutLosingInstallation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, finish := make(chan struct{}), make(chan struct{})
		probes, installs, state := 0, 0, "UPDATE_REQUIRED"
		b := &inputRefresh{inspect: func() string {
			probes++
			if probes == 1 {
				close(entered)
				<-finish
			}
			return state
		}, run: func(context.Context) error { installs++; state = "READY"; return nil }}
		b.status()
		<-entered
		result := make(chan error, 1)
		go func() { result <- b.automatic(context.Background()) }()
		synctest.Wait()
		select {
		case err := <-result:
			t.Fatal("startup lost to renderer health check", err)
		default:
		}
		close(finish)
		if err := <-result; err != nil || installs != 1 || b.status() != "READY" {
			t.Fatal(err, installs, b.status())
		}
	})
}

func TestHealthReaderWaitHonorsCancelledPreparation(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	b := &inputRefresh{inspect: func() string { close(entered); <-finish; return "READY" }, run: func(context.Context) error { t.Fatal("cancelled installation ran"); return nil }}
	b.status()
	<-entered
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if !errors.Is(b.automatic(ctx), context.Canceled) {
		t.Fatal("cancelled preparation waited for reader")
	}
	close(finish)
	b.mu.Lock()
	b.mu.Unlock()
}

func TestRestorationCheckCancellationKeepsSingleReadOnlyWorker(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	var probes atomic.Int32
	b := &inputRefresh{inspect: func() string { probes.Add(1); close(entered); <-finish; return "READY" },
		run: func(context.Context) error { t.Fatal("health observation invoked installer"); return nil }}
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan error, 1)
	go func() { result <- b.restoreCheck(ctx) }()
	<-entered
	cancel()
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatal("restoration did not stop waiting", err)
	}
	for range 3 {
		if err := b.restoreCheck(ctx); !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
		b.status()
		b.recheck()
	}
	if probes.Load() != 1 || !errors.Is(b.explicit(context.Background()), ErrStartupBusy) {
		t.Fatal("cancelled wait duplicated reader or allowed installer", probes.Load())
	}
	close(finish)
	b.mu.Lock()
	b.mu.Unlock()
	if b.status() != "READY" {
		t.Fatal("read-only worker did not publish result")
	}
}

func TestRestorationWaitSharesExistingReaderAndDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		entered, finish := make(chan struct{}), make(chan struct{})
		probes := 0
		b := &inputRefresh{inspect: func() string { probes++; close(entered); <-finish; return "READY" }}
		b.status()
		<-entered
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		result := make(chan error, 1)
		go func() { result <- b.restoreCheck(ctx) }()
		time.Sleep(2 * time.Second)
		if err := <-result; !errors.Is(err, context.DeadlineExceeded) || probes != 1 {
			t.Fatal(err, probes)
		}
		ready := make(chan error, 1)
		go func() { ready <- b.restoreCheck(context.Background()) }()
		synctest.Wait()
		close(finish)
		if err := <-ready; err != nil || probes != 1 {
			t.Fatal("late waiter did not share observation", err, probes)
		}
	})
}

func TestDisconnectCompletesWhileRestorationHealthReadIsBlocked(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	b.updatedResume = nil
	c.state = "CONNECTED"
	b.RefreshProtectedInput(func(context.Context) error { return &inputMaintenancePending{} })
	entered, finish := make(chan struct{}), make(chan struct{})
	r := &inputRefresh{inspect: func() string { close(entered); <-finish; return "READY" }}
	s := Startup{InputRestoreCheck: r.restoreCheck, Start: func() error { t.Fatal("started after disconnect"); return nil }}
	b.PendingInputSetup = func() string { return "READY" }
	b.PendingInputRestore = s.RestoreProtectedInput
	ticked := make(chan struct{})
	go func() { b.tick(now); close(ticked) }()
	<-entered
	disconnected := make(chan BridgeSnapshot, 1)
	go func() { disconnected <- b.Disconnect() }()
	select {
	case value := <-disconnected:
		if value.Code != "ok" || value.ReconnectCode != "" {
			t.Fatal(value)
		}
	case <-time.After(2 * time.Second):
		close(finish)
		t.Fatal("disconnect waited for slow health inspection")
	}
	<-ticked
	if b.pendingInput != nil || c.connects != 0 {
		t.Fatal("cancelled restoration connected")
	}
	close(finish)
	r.mu.Lock()
	r.mu.Unlock()
}
