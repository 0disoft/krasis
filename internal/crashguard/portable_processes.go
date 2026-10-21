package crashguard

import (
	"context"
	"errors"
	"io"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/syncgate"
)

// PortableProcesses owns distinct ordinary input and recovery children.
// Its serial gate preserves ledger/input command order across both streams.
type PortableProcesses struct {
	input, guardian *Process
	ownership       io.Closer
	gate            syncgate.Mutex
	closing         atomic.Bool
	inputFenced     atomic.Bool
	closeOnce       sync.Once
	closeDone       chan struct{}
	mu              sync.Mutex
	closeCause      error
	closeErr        error
	lifetimeOnce    sync.Once
	lifetimeDone    chan struct{}
}

func (processes *PortableProcesses) PreparePress(token control.PressedToken) (bool, error) {
	return processes.PreparePressContext(context.Background(), token)
}

func (processes *PortableProcesses) PreparePressContext(ctx context.Context, token control.PressedToken) (bool, error) {
	var prepared bool
	err := processes.invoke(ctx, func(ctx context.Context) error {
		return operationFailure("prepare", processes.guardian.invokeContext(ctx, func() error {
			var err error
			prepared, err = processes.guardian.client.PreparePress(token)
			return err
		}, false))
	})
	if err != nil {
		return false, err
	}
	return prepared, err
}

func (processes *PortableProcesses) CancelPreparedPress(token control.PressedToken) error {
	return processes.invoke(context.Background(), func(ctx context.Context) error {
		return operationFailure("cancel-prepare", processes.guardian.invokeContext(ctx, func() error { return processes.guardian.client.CancelPreparedPress(token) }, false))
	})
}

func (processes *PortableProcesses) Released(token control.PressedToken) error {
	return processes.invoke(context.Background(), func(ctx context.Context) error {
		return operationFailure("release", processes.guardian.invokeContext(ctx, func() error { return processes.guardian.client.Released(token) }, false))
	})
}

func (processes *PortableProcesses) ExchangeInput(ctx context.Context, exchange func(io.Writer, io.Reader) error) error {
	return processes.invoke(ctx, func(ctx context.Context) error {
		if processes.inputFenced.Load() || processes.guardian.failed.Load() || processes.guardian.closing.Load() {
			return ErrUnexpectedExit
		}
		return processes.input.ExchangeInput(ctx, exchange)
	})
}

func (processes *PortableProcesses) invoke(parent context.Context, operation func(context.Context) error) error {
	if processes == nil || processes.input == nil || processes.guardian == nil || processes.guardian.client == nil || parent == nil || operation == nil {
		return ErrUnexpectedExit
	}
	ctx, cancel := context.WithTimeout(parent, processIPCTimeout)
	defer cancel()
	if err := processes.gate.LockContext(ctx); err != nil {
		return errors.Join(ErrIPCTimeout, err)
	}
	defer processes.gate.Unlock()
	if processes.closing.Load() {
		return ErrUnexpectedExit
	}
	select {
	case <-processes.input.Done():
		return ErrUnexpectedExit
	case <-processes.guardian.Done():
		return ErrUnexpectedExit
	default:
	}
	err := operation(ctx)
	if err != nil {
		processes.inputFenced.Store(true)
	}
	return err
}

// Done is the live-session fault signal, not the full retirement receipt.
func (processes *PortableProcesses) Done() <-chan struct{} {
	if processes == nil || processes.input == nil || processes.guardian == nil {
		return nil
	}
	processes.lifetimeOnce.Do(func() {
		processes.lifetimeDone = make(chan struct{})
		go func() {
			select {
			case <-processes.input.Done():
			case <-processes.guardian.Done():
			}
			close(processes.lifetimeDone)
		}()
	})
	return processes.lifetimeDone
}

// Close acknowledges retirement of BOTH children and all their native/IPC
// tasks. It never joins two unrelated ExitPending errors with separate Done
// channels; the one outer receipt covers the complete ordered retirement.
func (processes *PortableProcesses) Close() error {
	if processes == nil {
		return nil
	}
	processes.closeOnce.Do(func() {
		processes.closing.Store(true)
		processes.closeDone = make(chan struct{})
		go func() {
			processes.gate.Lock()
			processes.gate.Unlock()
			inputErr := processes.retireChild(processes.input)
			guardErr := processes.retireChild(processes.guardian)
			processes.closeErr = errors.Join(inputErr, guardErr)
			if processes.ownership != nil {
				processes.closeErr = errors.Join(processes.closeErr, processes.ownership.Close())
			}
			close(processes.closeDone)
		}()
	})
	timer := time.NewTimer(2 * processIPCTimeout)
	defer timer.Stop()
	select {
	case <-processes.closeDone:
		return operationFailure("portable-close", processes.closeErr)
	case <-timer.C:
		select {
		case <-processes.closeDone:
			return operationFailure("portable-close", processes.closeErr)
		default:
		}
		processes.mu.Lock()
		cause := errors.Join(ErrExitTimeout, processes.closeCause)
		processes.mu.Unlock()
		cause = errors.Join(cause, childRetirementCause(processes.input), childRetirementCause(processes.guardian))
		return operationFailure("portable-close", &ExitPending{Done: processes.closeDone, Cause: cause})
	}
}

func (processes *PortableProcesses) retireChild(child *Process) error {
	if child == nil {
		return nil
	}
	err := child.Close()
	var pending *ExitPending
	if errors.As(err, &pending) {
		processes.recordCloseCause(pending.Cause)
		<-pending.Done
		// The same child owns this join. Close observes its final result and
		// cannot start another native Kill, pipe close or protocol operation.
		err = child.Close()
	}
	processes.recordCloseCause(err)
	return err
}

func (processes *PortableProcesses) recordCloseCause(err error) {
	processes.mu.Lock()
	processes.closeCause = errors.Join(processes.closeCause, err)
	processes.mu.Unlock()
}

func childRetirementCause(child *Process) error {
	if child == nil {
		return nil
	}
	child.mu.Lock()
	defer child.mu.Unlock()
	return errors.Join(child.closeCause, child.abortErr, child.stdinErr, child.stdoutErr, child.waitErr)
}
