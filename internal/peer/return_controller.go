package peer

import (
	"context"
	"errors"
	"fmt"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/transport"
)

type ReturnController struct {
	Coordinator *SessionWriteCoordinator
	Runtime     *application.SessionRuntime
	Returns     ReturnBoundaryPort
	Observer    ControlFlowObserver
	Cursor      ReturnCursorPresentation
}

// ReturnCursorPresentation affects only the idle pointer, not control ownership.
// The prepared action must be invalidated by local input or a new entry.
type ReturnCursorPresentation interface {
	PrepareReturnCursor() func()
	RestoreReturnCursor()
}

func NewReturnController(
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	returns ReturnBoundaryPort,
) (*ReturnController, error) {
	return NewReturnControllerObserved(coordinator, runtime, returns, nil)
}

func NewReturnControllerObserved(
	coordinator *SessionWriteCoordinator,
	runtime *application.SessionRuntime,
	returns ReturnBoundaryPort,
	observer ControlFlowObserver,
) (*ReturnController, error) {
	if coordinator == nil || runtime == nil || returns == nil {
		return nil, errors.New("return write coordinator, runtime, and boundary lifecycle are required")
	}
	return &ReturnController{Coordinator: coordinator, Runtime: runtime, Returns: returns, Observer: observer}, nil
}

func (controller *ReturnController) Revoke(ctx context.Context, intent control.ReturnIntent) error {
	if controller == nil || controller.Coordinator == nil || controller.Runtime == nil || controller.Returns == nil {
		return errors.New("return controller is not initialized")
	}
	var emitProfile func()
	err := controller.Coordinator.Execute(ctx, func(writer FrameWriter) (operationErr error) {
		defer func() {
			if operationErr == nil {
				emitProfile = handoff.Retire(handoff.Matching(ctx, "receiver", intent.SessionID, intent.Epoch), "ok")
			}
		}()
		var present func()
		if controller.Cursor != nil {
			present = controller.Cursor.PrepareReturnCursor()
		}
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
		if err := writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload}); err != nil {
			observeControlFlow(controller.Observer, ControlFlowRevokeWriteOutcomeUnknown)
			return fmt.Errorf("%w: %w", ErrInboundRevokeWrite, err)
		}
		observeControlFlow(controller.Observer, ControlFlowRevokeWriteCompleted)
		if present != nil {
			present()
		}
		return nil
	})
	if errors.Is(err, application.ErrRuntimeInputHeld) {
		controller.Returns.RearmReturn(intent)
	}
	if emitProfile != nil {
		emitProfile()
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
