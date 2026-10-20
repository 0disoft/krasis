package crashguard

import (
	"bytes"
	"errors"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

type stalledGuardPipe struct {
	bytes.Buffer
	release <-chan struct{}
	closes  atomic.Int32
}

func (pipe *stalledGuardPipe) Close() error {
	pipe.closes.Add(1)
	<-pipe.release
	return nil
}

func TestGuardShutdownBoundsBlockedKillAndPipeClose(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := closeTestProcess(t)
		releaseKill, releasePipes := make(chan struct{}), make(chan struct{})
		in, out := &stalledGuardPipe{release: releasePipes}, &stalledGuardPipe{release: releasePipes}
		p.stdin, p.stdout = in, out
		var kills atomic.Int32
		p.kill = func() error { kills.Add(1); <-releaseKill; return nil }
		started := time.Now()
		err := p.Close()
		var pending *ExitPending
		if !errors.As(err, &pending) || !errors.Is(err, ErrExitTimeout) || time.Since(started) != 2*processIPCTimeout {
			t.Fatal("native cleanup defeated the shutdown budget", err, time.Since(started))
		}
		synctest.Wait()
		if kills.Load() != 1 || in.closes.Load() != 1 || out.closes.Load() != 1 {
			t.Fatal("native cleanup tasks were not started exactly once")
		}
		// Actual child exit must cancel the live session, but cannot allow a
		// replacement while native cleanup still owns its old handles.
		close(p.done)
		select {
		case <-p.Done():
		default:
			t.Fatal("real child exit was hidden from the session monitor")
		}
		select {
		case <-pending.Done:
			t.Fatal("child exit bypassed pending native cleanup")
		default:
		}
		results := make(chan error, 8)
		for range cap(results) {
			go func() { results <- p.Close() }()
		}
		for range cap(results) {
			var again *ExitPending
			if err := <-results; !errors.As(err, &again) || again.Done != pending.Done {
				t.Fatal("repeated Close replaced the retirement owner", err)
			}
		}
		if kills.Load() != 1 || in.closes.Load() != 1 || out.closes.Load() != 1 {
			t.Fatal("repeated Close duplicated blocked native work")
		}
		close(releaseKill)
		synctest.Wait()
		select {
		case <-pending.Done:
			t.Fatal("completed Kill bypassed unjoined pipe cleanup")
		default:
		}
		close(releasePipes)
		<-pending.Done
		var again *ExitPending
		if err := p.Close(); errors.As(err, &again) || !errors.Is(err, ErrExitTimeout) {
			t.Fatal("retirement lost the original shutdown failure", err)
		}
	})
}

func TestGuardProtocolTimeoutDoesNotJoinNativeAbort(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		p := closeTestProcess(t)
		releaseOperation, releaseKill := make(chan struct{}), make(chan struct{})
		p.kill = func() error { <-releaseKill; return nil }
		operationDone := make(chan struct{})
		started := time.Now()
		err := p.invoke(func() error { <-releaseOperation; close(operationDone); return nil })
		if !errors.Is(err, ErrIPCTimeout) || time.Since(started) != processIPCTimeout {
			t.Fatal("protocol timeout waited for native termination", err)
		}
		close(p.done)
		var pending *ExitPending
		if err := p.Close(); !errors.As(err, &pending) {
			t.Fatal("unfinished protocol/abort was reported retired", err)
		}
		close(releaseKill)
		synctest.Wait()
		select {
		case <-pending.Done:
			t.Fatal("retirement bypassed the still-running protocol call")
		default:
		}
		close(releaseOperation)
		<-operationDone
		<-pending.Done
		if p.invoke(func() error { t.Error("late completion revived protocol"); return nil }) == nil {
			t.Fatal("retired guard admitted another call")
		}
	})
}
