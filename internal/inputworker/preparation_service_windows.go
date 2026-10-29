//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"
)

const preparationPipeName = `\\.\pipe\Krasis.Preparation.v1`

// A passive cursor owns the serial input endpoint until its exact retirement.
// Fresh movement may still request one unbound replacement here. This endpoint
// accepts no lease, input, agent handle transfer or expiry extension.
func runPreparationService(ctx context.Context, installation *Installation, admit func(context.Context) error, slot *serviceInputPreparation) (result error) {
	agent, service, err := installation.digests()
	if err != nil {
		return err
	}
	listener, err := newServicePipeListener(preparationPipeName, nil)
	if err != nil {
		return err
	}
	defer func() { result = errors.Join(result, listener.Close()) }()
	for ctx.Err() == nil {
		initial, err := listener.borrow()
		if err != nil {
			return err
		}
		channel, err := acceptAgentNamed(ctx, agent, nil, installation, initial, preparationPipeName)
		if err == nil {
			err = receivePreparationDemand(ctx, channel, admit)
			closeErr := channel.Close()
			if failure := serviceRecoveryFailure(err, closeErr); failure != nil {
				return failure
			}
			if err == nil && closeErr == nil {
				slot.hint(ctx, time.Time{}, time.Now(), func(run context.Context) (*preparedConsole, error) {
					return prepareServiceConsole(run, installation, service)
				})
				if err := slot.collectReady(); err != nil {
					return err
				}
				continue
			}
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(200 * time.Millisecond):
		}
	}
	return nil
}

func receivePreparationDemand(ctx context.Context, channel *Channel, admit func(context.Context) error) error {
	_, prepare, err := receiveAgentRequest(ctx, channel)
	if err != nil {
		return err
	}
	if !prepare {
		return ErrProtocol
	}
	if admit != nil {
		if err := admit(ctx); err != nil {
			return err
		}
	}
	return acknowledgePreparation(ctx, channel)
}
