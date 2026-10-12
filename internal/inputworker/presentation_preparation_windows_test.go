//go:build windows

package inputworker

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"golang.org/x/sys/windows"
)

func TestCursorInputPreparationDoesNotBlockAndTransfersOnce(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		started, release := make(chan struct{}), make(chan struct{})
		child := &preparedConsole{}
		pending := startCursorInputPreparation(context.Background(), func(context.Context) (*preparedConsole, error) {
			close(started)
			<-release
			return child, nil
		})
		<-started
		select {
		case <-pending.done:
			t.Fatal("preparation completed without builder release")
		default:
		}
		close(release)
		got, err := pending.join()
		if got != child || err != nil {
			t.Fatal("prepared child lost", err)
		}
		if got, err := pending.join(); got != nil || err != nil {
			t.Fatal("prepared child transferred twice", err)
		}
		if child.close() != nil {
			t.Fatal("transferred child cleanup failed")
		}
	})
}

func TestCursorInputPreparationCancellationTimeoutAndCleanupFence(t *testing.T) {
	for _, kind := range []string{"cancel", "timeout", "cleanup failure"} {
		t.Run(kind, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				child := &preparedConsole{}
				if kind == "cleanup failure" {
					child.worker = &ConsoleWorker{job: windows.Handle(0x7ffffffe), done: make(chan struct{})}
				}
				pending := startCursorInputPreparation(ctx, func(run context.Context) (*preparedConsole, error) {
					<-run.Done()
					return child, nil
				})
				want := context.Canceled
				if kind == "timeout" {
					time.Sleep(startupTimeout + time.Millisecond)
					want = context.DeadlineExceeded
				} else {
					cancel()
				}
				got, err := pending.join()
				if got != nil || !errors.Is(err, want) || child.warmState.Load() != 2 {
					t.Fatal("cancelled preparation retained child/ownership", err)
				}
				if kind == "cleanup failure" && !errors.Is(err, ErrRecovery) {
					t.Fatal("native cleanup error did not fence future admission", err)
				}
			})
		})
	}
}
