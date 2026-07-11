package peer

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type ReturnController struct {
	Writer  FrameWriter
	Runtime *application.SessionRuntime
	Returns ReturnBoundaryPort
}

func NewReturnController(
	writer FrameWriter,
	runtime *application.SessionRuntime,
	returns ReturnBoundaryPort,
) (*ReturnController, error) {
	if writer == nil || runtime == nil || returns == nil {
		return nil, errors.New("return frame writer, runtime, and boundary lifecycle are required")
	}
	return &ReturnController{Writer: writer, Runtime: runtime, Returns: returns}, nil
}

func (controller *ReturnController) Revoke(ctx context.Context, intent control.ReturnIntent) error {
	if controller == nil || controller.Writer == nil || controller.Runtime == nil || controller.Returns == nil {
		return errors.New("return controller is not initialized")
	}
	identity, _, err := controller.Runtime.RevokeLocalReturn(ctx, intent)
	if err != nil {
		if errors.Is(err, application.ErrRuntimeInputHeld) {
			controller.Returns.RearmReturn(intent)
		}
		return err
	}
	defer controller.Returns.DisarmReturn(intent)
	payload, err := transport.EncodeControlPayload(transport.ControlCommand{
		Kind:      transport.ControlLeaseRevoke,
		SessionID: identity.SessionID,
		LeaseID:   identity.LeaseID,
		Epoch:     identity.Epoch,
	})
	if err != nil {
		return err
	}
	return controller.Writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload})
}

func (controller *ReturnController) Run(ctx context.Context, intents <-chan control.ReturnIntent) error {
	if controller == nil || controller.Writer == nil || controller.Runtime == nil || controller.Returns == nil {
		return errors.New("return controller is not initialized")
	}
	if ctx == nil || intents == nil {
		return errors.New("return context and intent channel are required")
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case intent, ok := <-intents:
			if !ok {
				return nil
			}
			if err := controller.Revoke(ctx, intent); err != nil {
				if errors.Is(err, application.ErrStaleReturnIntent) || errors.Is(err, application.ErrRuntimeNotInbound) {
					continue
				}
				if errors.Is(err, application.ErrRuntimeInputHeld) {
					continue
				}
				return err
			}
		}
	}
}
