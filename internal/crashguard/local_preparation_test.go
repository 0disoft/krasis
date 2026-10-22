package crashguard

import (
	"context"
	"errors"
	"os/exec"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestPreparedLocalInputBindsOnceWithoutEarlyBytes(t *testing.T) {
	owner := closeTestProcess(t)
	prepared, err := prepareLocalInputCommand(context.Background(), nil, func(*exec.Cmd) (*Process, error) { return owner, nil })
	if err != nil || owner.stdin.(*closeTestPipe).Len() != 0 {
		t.Fatal("preparation published bootstrap bytes", err)
	}
	bound, err := prepared.Bind(context.Background(), []byte("bootstrap"))
	if err != nil || bound != owner || owner.stdin.(*closeTestPipe).String() != "bootstrap" {
		t.Fatal("bootstrap ownership transfer failed", err)
	}
	if _, err := prepared.Bind(context.Background(), []byte("replayed")); err == nil {
		t.Fatal("preparation bound twice")
	}
	if err := prepared.Close(); err != nil || owner.closing.Load() {
		t.Fatal("preparation closed its transferred transport", err)
	}
	close(owner.done)
	if err := owner.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedLocalInputCancellationRetiresWithoutDisarmOrBootstrap(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		owner := closeTestProcess(t)
		var kills atomic.Int32
		owner.kill = func() error { kills.Add(1); close(owner.done); return nil }
		prepared, err := prepareLocalInputCommand(ctx, nil, func(*exec.Cmd) (*Process, error) { return owner, nil })
		if err != nil {
			t.Fatal(err)
		}
		cancel()
		if _, err := prepared.Bind(ctx, []byte("bootstrap")); !errors.Is(err, context.Canceled) {
			t.Fatal("canceled bind accepted", err)
		}
		if err := prepared.Close(); err != nil || kills.Load() != 1 || owner.stdin.(*closeTestPipe).Len() != 0 {
			t.Fatal("unbound cleanup wrote disarm or lost ownership", err, kills.Load())
		}
	})
}

func TestPreparedLocalInputKeepsUnconfirmedCleanupFenced(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		owner := closeTestProcess(t)
		denied := errors.New("kill denied")
		owner.kill = func() error { return denied }
		prepared := &LocalInputPreparation{process: owner}
		err := prepared.Close()
		var pending *ExitPending
		if !errors.As(err, &pending) || !errors.Is(err, denied) {
			t.Fatal("unconfirmed exit lost its exact cleanup receipt", err)
		}
		if _, err := prepared.Bind(context.Background(), []byte("bootstrap")); err == nil || owner.stdin.(*closeTestPipe).Len() != 0 {
			t.Fatal("retiring child received a bootstrap", err)
		}
		close(owner.done)
		<-pending.Done
		if !errors.Is(prepared.Close(), denied) {
			t.Fatal("late exit erased native cleanup failure")
		}
	})
}

func TestPreparedLocalInputRetiresLateLaunch(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
		defer cancel()
		owner := closeTestProcess(t)
		owner.kill = func() error { close(owner.done); return nil }
		prepared, err := prepareLocalInputCommand(ctx, nil, func(*exec.Cmd) (*Process, error) {
			time.Sleep(2 * time.Millisecond)
			return owner, nil
		})
		if prepared != nil || !errors.Is(err, context.DeadlineExceeded) {
			t.Fatal("late launch escaped ownership", err)
		}
		select {
		case <-owner.closeDone:
		default:
			t.Fatal("late launch not joined")
		}
	})
}

func TestUnboundWaitFailurePreservesNativeFailures(t *testing.T) {
	native := errors.New("wait failed")
	if unboundWaitFailure(errors.Join(&exec.ExitError{})) != nil {
		t.Fatal("expected unbound termination became recovery failure")
	}
	if !errors.Is(unboundWaitFailure(errors.Join(native, &exec.ExitError{})), native) {
		t.Fatal("native wait failure erased with the exit code")
	}
}
