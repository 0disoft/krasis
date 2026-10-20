package crashguard

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestPortableProcessesRetirementRetainsBothChildrenAndAllCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input, guardian := closeTestProcess(t), closeTestProcess(t)
		var ownershipCloses atomic.Int32
		ownershipErr := errors.New("ownership close failed")
		processes := &PortableProcesses{input: input, guardian: guardian, ownership: ownershipCloseFunc(func() error { ownershipCloses.Add(1); return ownershipErr })}
		inputCleanup := make(chan struct{})
		input.stdin = &stalledGuardPipe{release: inputCleanup}
		inputDenied, guardDenied := errors.New("input termination denied"), errors.New("guardian termination denied")
		var inputKills, guardKills atomic.Int32
		input.kill = func() error { inputKills.Add(1); return inputDenied }
		guardian.kill = func() error { guardKills.Add(1); return guardDenied }
		var pending *ExitPending
		started := time.Now()
		if err := processes.Close(); !errors.As(err, &pending) || time.Since(started) != 2*processIPCTimeout {
			t.Fatal("pair shutdown lost its caller budget/full receipt", err)
		}
		// Even exact input exit cannot disarm G while I's native cleanup owns
		// old resources. Its live signal must not grant replacement either.
		close(input.done)
		<-processes.Done()
		synctest.Wait()
		if guardian.closing.Load() || guardKills.Load() != 0 {
			t.Fatal("guardian disarmed before full input retirement")
		}
		if ownershipCloses.Load() != 0 {
			t.Fatal("input exit released pair ownership before native cleanup")
		}
		select {
		case <-pending.Done:
			t.Fatal("input exit fabricated complete pair retirement")
		default:
		}
		close(inputCleanup)
		synctest.Wait()
		// The second child can remain pending independently. All callers
		// retain the SAME outer receipt rather than errors.As picking I only.
		for range 4 {
			var again *ExitPending
			err := processes.Close()
			if !errors.As(err, &again) || again.Done != pending.Done || !errors.Is(err, inputDenied) || !errors.Is(err, guardDenied) {
				t.Fatal("second pending child lost first child's cause or combined receipt", err)
			}
		}
		if inputKills.Load() != 1 || guardKills.Load() != 1 {
			t.Fatal("repeated pair Close duplicated native termination")
		}
		if ownershipCloses.Load() != 0 {
			t.Fatal("pending guardian released pair ownership")
		}
		close(guardian.done)
		<-pending.Done
		err := processes.Close()
		var incomplete *ExitPending
		if errors.As(err, &incomplete) || !errors.Is(err, inputDenied) || !errors.Is(err, guardDenied) {
			t.Fatal("completed pair hid child errors or retained a stale pending wrapper", err)
		}
		if !errors.Is(err, ownershipErr) || ownershipCloses.Load() != 1 {
			t.Fatal("ownership close error or once-only lifetime lost", err, ownershipCloses.Load())
		}
		_ = processes.Close()
		if ownershipCloses.Load() != 1 {
			t.Fatal("ownership close retried a possibly reused handle")
		}
	})
}

type ownershipCloseFunc func() error

func (close ownershipCloseFunc) Close() error { return close() }

func TestPortableProcessesAdmissionAndExchangeShareOneBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		input, guardian := closeTestProcess(t), closeTestProcess(t)
		input.portable = true
		processes := &PortableProcesses{input: input, guardian: guardian}
		processes.gate.Lock()
		go func() { time.Sleep(processIPCTimeout * 3 / 4); processes.gate.Unlock() }()
		release := make(chan struct{})
		started := time.Now()
		err := processes.ExchangeInput(context.Background(), func(io.Writer, io.Reader) error { <-release; return nil })
		if !errors.Is(err, ErrIPCTimeout) || time.Since(started) != processIPCTimeout {
			t.Fatal("exchange restarted its budget after pair admission", time.Since(started), err)
		}
		close(release)
		close(input.done)
		close(guardian.done)
		_ = processes.Close()
	})
}

func TestPortableProcessesRejectInputAfterFailureAndShutdown(t *testing.T) {
	input, guardian := closeTestProcess(t), closeTestProcess(t)
	input.portable = true
	processes := &PortableProcesses{input: input, guardian: guardian}
	failure := errors.New("lost input receipt")
	if err := processes.ExchangeInput(context.Background(), func(io.Writer, io.Reader) error { return failure }); !errors.Is(err, failure) {
		t.Fatal(err)
	}
	if err := processes.ExchangeInput(context.Background(), func(io.Writer, io.Reader) error { t.Error("failed pair replayed input"); return nil }); err == nil {
		t.Fatal("failed pair accepted another input owner")
	}
	close(input.done)
	close(guardian.done)
	if err := processes.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := processes.PreparePress(control.PressedToken{Kind: control.PressedKeyboardKey, Code: 30}); err == nil {
		t.Fatal("closed pair changed its journal")
	}
}
