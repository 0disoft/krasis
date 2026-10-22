package crashguard

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestLocalInputStartupRejectsCanceledCallerBeforeLaunch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p, err := startLocalInputCommand(ctx, nil, nil, func(*exec.Cmd) (*Process, error) {
		t.Fatal("canceled request launched a native child")
		return nil, nil
	})
	if p != nil || !errors.Is(err, context.Canceled) {
		t.Fatal("startup lost caller cancellation", p, err)
	}
}

func TestLocalInputStartupRetiresLaunchThatFinishesAfterCancellation(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		owner := closeTestProcess(t)
		var kills atomic.Int32
		owner.kill = func() error { kills.Add(1); close(owner.done); return nil }
		p, err := startLocalInputCommand(ctx, nil, []byte("bootstrap"), func(*exec.Cmd) (*Process, error) {
			time.Sleep(20 * time.Millisecond) // A native launch cannot be interrupted.
			return owner, nil
		})
		if p != owner || !errors.Is(err, context.DeadlineExceeded) || kills.Load() != 1 || owner.stdin.(*closeTestPipe).Len() != 0 {
			t.Fatal("late launch received data or escaped retirement", p, err, kills.Load())
		}
		select {
		case <-owner.closeDone:
		default:
			t.Fatal("late native owner was not fully joined")
		}
	})
}

func TestLocalInputBootstrapUsesRemainingCallerBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
		defer cancel()
		owner := closeTestProcess(t)
		reader, writer := io.Pipe() // Child never consumes bootstrap bytes.
		defer reader.Close()
		owner.stdin = writer
		owner.kill = func() error { close(owner.done); return nil }
		started := time.Now()
		p, err := startLocalInputCommand(ctx, nil, []byte("bootstrap"), func(*exec.Cmd) (*Process, error) {
			return owner, nil
		})
		if p != owner || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 10*time.Millisecond {
			t.Fatal("bootstrap replaced the caller deadline with its own timeout", err, time.Since(started))
		}
		select {
		case <-owner.closeDone:
		default:
			t.Fatal("blocked bootstrap owner survived startup failure")
		}
	})
}
