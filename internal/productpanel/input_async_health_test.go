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
