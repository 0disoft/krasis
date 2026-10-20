package crashguard

import (
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestTimedOutGuardNeverStartsAnotherProtocolCall(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := closeTestProcess(t)
		entered, release, late := make(chan struct{}), make(chan struct{}), make(chan struct{})
		var calls, kills atomic.Int32
		denied := errors.New("termination unconfirmed")
		p.kill = func() error { kills.Add(1); return denied }
		done := make(chan error, 1)
		go func() {
			done <- p.invoke(func() error {
				calls.Add(1)
				close(entered)
				<-release // Even pipe Close/process Kill cannot join this call.
				close(late)
				return nil
			})
		}()
		<-entered
		waiters := make(chan error, 20)
		for range cap(waiters) {
			go func() { waiters <- p.invoke(func() error { calls.Add(1); return nil }) }()
		}
		if err := <-done; !errors.Is(err, ErrIPCTimeout) {
			t.Fatal("protocol timeout lost its failure", err)
		}
		<-p.requestAbort()
		for range cap(waiters) {
			if err := <-waiters; !errors.Is(err, ErrIPCTimeout) {
				t.Fatal("contended guard request bypassed admission", err)
			}
		}
		for range 20 {
			if err := p.invoke(func() error { calls.Add(1); return nil }); !errors.Is(err, ErrIPCTimeout) {
				t.Fatal("retired guard accepted another operation", err)
			}
		}
		if calls.Load() != 1 || kills.Load() != 1 {
			t.Fatal("stuck protocol spawned more operations or retried termination")
		}
		select {
		case <-p.Done():
			t.Fatal("timeout fabricated process-exit evidence")
		default:
		}
		close(release)
		<-late
		if p.invoke(func() error { t.Error("late success revived guard"); return nil }) == nil {
			t.Fatal("late result authorized another protocol call")
		}
		close(p.done)
	})
}

func TestGuardAdmissionDeadlineDoesNotStartOrAbortProtocolOwner(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := closeTestProcess(t)
		p.kill = func() error { t.Error("admission waiter killed protocol owner"); return nil }
		p.callMu.Lock()
		started := time.Now()
		if err := p.invoke(func() error { t.Error("expired admission started protocol call"); return nil }); !errors.Is(err, ErrIPCTimeout) || time.Since(started) != processIPCTimeout || p.failed.Load() {
			t.Fatal("admission lost its budget or retired an unrelated owner", err)
		}
		p.callMu.Unlock()
		if err := p.invoke(func() error { return nil }); err != nil {
			t.Fatal("expired admission prevented a fresh protocol call", err)
		}
		close(p.done)
	})
}
