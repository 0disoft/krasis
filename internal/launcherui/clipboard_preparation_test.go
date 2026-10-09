package launcherui

import (
	"context"
	"sync/atomic"
	"testing"
	"time"
)

func TestClipboardPreparationCancelAndLateResultDiscard(t *testing.T) {
	for _, change := range []bool{false, true} {
		ctx, cancel := context.WithCancel(context.Background())
		var current atomic.Bool
		current.Store(true)
		entered, done := make(chan struct{}), make(chan error, 1)
		go func() {
			_, err := prepareClipboardFiles(ctx, current.Load, func(ctx context.Context) ([]byte, error) {
				close(entered)
				<-ctx.Done() // A killed I/O helper may still return a successful payload.
				return []byte("late private payload"), nil
			})
			done <- err
		}()
		<-entered
		if change {
			current.Store(false)
		} else {
			cancel()
		}
		select {
		case err := <-done:
			if err == nil {
				t.Fatal("late payload published")
			}
		case <-time.After(time.Second):
			t.Fatal("preparation blocked sharing-off or shutdown")
		}
		cancel()
	}
}
