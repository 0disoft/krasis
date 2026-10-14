//go:build windows

package residentagent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/wininput"
)

func TestProtectedCursorPromotionWaitsForIdleAndJoinsOldOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		var protected atomic.Bool
		native := &retiringSecureTest{}
		native.active.Store(true)
		cleaning, release, captured := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var releaseOnce sync.Once
		defer func() {
			cancel()
			releaseOnce.Do(func() { close(release) })
			synctest.Wait()
		}()
		native.run = func(attempt context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, ready func()) error {
			ready()
			<-attempt.Done()
			close(cleaning)
			<-release
			ready() // Old readiness cannot reopen the replacement's admission.
			return attempt.Err()
		}
		secure := &retiringSecureTest{run: func(attempt context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, ready func()) error {
			ready()
			close(captured)
			<-attempt.Done()
			return attempt.Err()
		}}
		h := &protectedSource{native: native, blocked: func() bool { return false }, cursorProtected: protected.Load}
		h.SetProtectedSource(secure)
		var ready, promoted atomic.Int32
		h.event = func(code string) {
			if code == "installed_source_cursor_promoted" {
				if !h.mu.TryLock() {
					t.Error("promotion observer retained source gate")
				} else {
					h.mu.Unlock()
				}
				promoted.Add(1)
			}
		}
		done := make(chan error, 1)
		go func() { done <- h.RunWithBoundary(ctx, &wininput.SourceSession{}, nil, func() { ready.Add(1) }) }()
		synctest.Wait()
		protected.Store(true)
		time.Sleep(3 * protectedCursorWatchInterval)
		synctest.Wait()
		if !h.captureReady() || promoted.Load() != 0 {
			t.Fatal("live capture was replaced")
		}
		native.active.Store(false)
		time.Sleep(protectedCursorWatchInterval)
		synctest.Wait()
		select {
		case <-cleaning:
		default:
			t.Fatal("idle protected foreground did not promote capture")
		}
		if h.captureReady() {
			t.Fatal("old capture remained admitted during cleanup")
		}
		select {
		case <-captured:
			t.Fatal("new native owner overlapped old cleanup")
		default:
		}
		releaseOnce.Do(func() { close(release) })
		synctest.Wait()
		select {
		case <-captured:
		default:
			t.Fatal("replacement owner missing")
		}
		if !h.captureReady() || ready.Load() != 1 || promoted.Load() != 1 || h.PrepareReturnCursor() == nil || !secure.cursor.Load() {
			t.Fatal("replacement did not reuse protected cursor or readied connection twice")
		}
		protected.Store(false)
		time.Sleep(3 * protectedCursorWatchInterval)
		synctest.Wait()
		if !h.remoteOwner || promoted.Load() != 1 {
			t.Fatal("foreground return churned capture owners")
		}
		cancel()
		synctest.Wait()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatal(err)
		}
	})
}
