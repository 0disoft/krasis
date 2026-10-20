package inputworker

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/crashguard"
)

// The authenticated serving worker retains its journal. Only the separate
// native child executes live commands. Recovery may use the worker's native
// release port only after exact child exit AND owned IPC retirement are proved.
type isolatedInput struct {
	*Client
	recovery   application.ReleasePort
	retire     func() (bool, error)
	recovering bool
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
