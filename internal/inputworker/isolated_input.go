package inputworker

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
	"sync"
	"time"
)

// The authenticated serving worker retains its journal. Only the separate
// native child executes live commands. Recovery starts only after exact input
// child/IPC retirement. Installed ordinary/login input uses a separate
// release-only child; direct test fixtures may supply their own recovery port.
type isolatedInput struct {
	*Client
	recovery          application.ReleasePort
	retire            func() (bool, error)
	recovering        bool
	recoveryRetire    func() (bool, error)
	recoveryCloseOnce sync.Once
	recoveryCloseErr  error
}

// Both native transports are prestarted before any journal can hold input.
// Recovery IPC is bounded and its process/pipe retirement is also mandatory.
func (input *isolatedInput) finishRecovery() error {
	if input.recoveryRetire == nil {
		return nil
	}
	input.recoveryCloseOnce.Do(func() {
		retired, err := input.recoveryRetire()
		if !retired || err != nil {
			input.recoveryCloseErr = errors.Join(err, ErrRecovery)
		}
	})
	return input.recoveryCloseErr
}

func recoveryExecution(input application.InputPort, owner executor) executor {
	isolated, ok := input.(*isolatedInput)
	if !ok || isolated.recoveryRetire == nil {
		return owner
	}
	return func(ctx context.Context, work func() error) error {
		if !isolated.recovering || ctx == nil || work == nil {
			return ErrRecovery
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		return work() // Native recovery checks execute only in the release child.
	}
}

func newIsolatedInput(client *Client, process *crashguard.Process, recovery application.ReleasePort) *isolatedInput {
	return &isolatedInput{Client: client, recovery: recovery, retire: func() (bool, error) {
		err := process.Close()
		var pending *crashguard.ExitPending
		if errors.As(err, &pending) {
			return false, err
		}
		select {
		case <-process.Done():
			return true, err
		default:
			return false, errors.Join(err, ErrRecovery)
		}
	}}
}

func (input *isolatedInput) retireForRecovery() (bool, error) {
	retired, err := input.retire()
	input.recovering = retired
	return retired, err
}

func (input *isolatedInput) Release(ctx context.Context, token control.PressedToken) error {
	if input.recovering {
		return input.recovery.Release(ctx, token)
	}
	return input.Client.Release(ctx, token)
}

// Only the concrete, independently authenticated native transport may move live
// execution out of the journal owner's desktop. The child performs fresh native
// authority/desktop checks for every command. Recovery has a distinct executor
// after exact child retirement; this is not cached authority.
func journalExecution(input application.InputPort, owner executor, timing *workerTiming) executor {
	isolated, ok := input.(*isolatedInput)
	if !ok {
		return owner
	}
	return beforeInputCertainty(func(ctx context.Context, operation func() error) error {
		if timing != nil {
			*timing = workerTiming{}
		}
		if isolated == nil || isolated.Client == nil || isolated.recovering || ctx == nil || operation == nil {
			return ErrAuthority
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		started := time.Now()
		err := operation()
		if timing != nil {
			// This measures IPC completion, not the child's individual native
			// queries. Leave authority/scope/desktop unobserved in this owner.
			timing.record(2, time.Since(started))
		}
		return err
	})
}
