package peer

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type ReturnController struct {
	Coordinator *SessionWriteCoordinator
	Runtime     *application.SessionRuntime
	Returns     ReturnBoundaryPort
}

func NewReturnController(
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	returns ReturnBoundaryPort,
) (*ReturnController, error) {
	if coordinator == nil || runtime == nil || returns == nil {
		return nil, errors.New("return write coordinator, runtime, and boundary lifecycle are required")
	}
	return &ReturnController{Coordinator: coordinator, Runtime: runtime, Returns: returns}, nil
}

func (controller *ReturnController) Revoke(ctx context.Context, intent control.ReturnIntent) error {
	if controller == nil || controller.Coordinator == nil || controller.Runtime == nil || controller.Returns == nil {
		return errors.New("return controller is not initialized")
	}
	err := controller.Coordinator.Execute(ctx, func(writer FrameWriter) error {
		identity, _, revokeErr := controller.Runtime.RevokeLocalReturn(ctx, intent)
		if revokeErr != nil {
			return revokeErr
		}
		defer controller.Returns.DisarmReturn(intent)
		payload, encodeErr := transport.EncodeControlPayload(transport.ControlCommand{
			Kind:      transport.ControlLeaseRevoke,
			SessionID: identity.SessionID,
			LeaseID:   identity.LeaseID,
			Epoch:     identity.Epoch,
		})
		if encodeErr != nil {
			return encodeErr
		}
		return writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload})
	})
	if errors.Is(err, application.ErrRuntimeInputHeld) {
		controller.Returns.RearmReturn(intent)
	}
	return err
}

func (controller *ReturnController) Run(ctx context.Context, intents <-chan control.ReturnIntent) error {
	if controller == nil || controller.Coordinator == nil || controller.Runtime == nil || controller.Returns == nil {
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
