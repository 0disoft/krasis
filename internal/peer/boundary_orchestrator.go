package peer

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"

	"github.com/0disoft/krasis/internal/control"
)

const outboundIdentifierBytes = 16

var ErrBoundaryIntentObserverPanic = errors.New("boundary intent observer panicked")

// A read-only local inspection could not prove admission. No lease or input
// has started, so discard this intent while keeping the authenticated session.
// Authority, lifecycle and transport failures must never carry this marker.
var ErrBoundaryInspectionUnavailable = errors.New("boundary inspection unavailable")

type OutboundIdentity struct {
	SessionID string
	Epoch     string
}

type OutboundIdentityFactory func() (OutboundIdentity, error)

type BoundaryBeginner interface {
	Ready() bool
	BeginBoundary(context.Context, control.BoundaryIntent, OutboundIdentity) error
}

type BoundaryAdmissionDecision uint8

const (
	BoundaryAdmissionAllowed BoundaryAdmissionDecision = iota + 1
	BoundaryAdmissionBlockedLocalMenu
	BoundaryAdmissionBlockedMouseCapture
	BoundaryAdmissionBlockedPressedInput
	BoundaryAdmissionBlockedMoveSize
)

func (decision BoundaryAdmissionDecision) Valid() bool {
	switch decision {
	case BoundaryAdmissionAllowed,
		BoundaryAdmissionBlockedLocalMenu,
		BoundaryAdmissionBlockedMouseCapture,
		BoundaryAdmissionBlockedPressedInput,
		BoundaryAdmissionBlockedMoveSize:
		return true
	default:
		return false
	}
}

type BoundaryAdmissionGate interface {
	AdmitBoundary(context.Context, control.BoundaryIntent) (BoundaryAdmissionDecision, error)
}

type BoundaryOrchestrator struct {
	beginner BoundaryBeginner
	identity OutboundIdentityFactory
	observer ControlFlowObserver
	gate     BoundaryAdmissionGate
}

func NewBoundaryOrchestrator(
	beginner BoundaryBeginner,
	identity OutboundIdentityFactory,
) (*BoundaryOrchestrator, error) {
	return NewBoundaryOrchestratorObserved(beginner, identity, nil)
}

func NewBoundaryOrchestratorObserved(
	beginner BoundaryBeginner,
	identity OutboundIdentityFactory,
	observer ControlFlowObserver,
) (*BoundaryOrchestrator, error) {
	return NewBoundaryOrchestratorObservedWithGate(beginner, identity, observer, nil)
}

func NewBoundaryOrchestratorObservedWithGate(
	beginner BoundaryBeginner,
	identity OutboundIdentityFactory,
	observer ControlFlowObserver,
	gate BoundaryAdmissionGate,
) (*BoundaryOrchestrator, error) {
	if beginner == nil || identity == nil {
		return nil, errors.New("boundary beginner and identity factory are required")
	}
	return &BoundaryOrchestrator{beginner: beginner, identity: identity, observer: observer, gate: gate}, nil
}

func (orchestrator *BoundaryOrchestrator) Run(
	ctx context.Context,
	intents <-chan control.BoundaryIntent,
) error {
	return orchestrator.RunWithIntentObserver(ctx, intents, nil)
}

// RunWithIntentObserver reports queued intents after they leave the hook's
// bounded channel, so diagnostic observers cannot delay the hook callback.
func (orchestrator *BoundaryOrchestrator) RunWithIntentObserver(
	ctx context.Context,
	intents <-chan control.BoundaryIntent,
	observeIntent func(control.BoundaryIntent),
) error {
	if orchestrator == nil || orchestrator.beginner == nil || orchestrator.identity == nil {
		return errors.New("boundary orchestrator is not initialized")
	}
	if ctx == nil || intents == nil {
		return errors.New("boundary context and intent channel are required")
	}
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case intent, ok := <-intents:
			if !ok {
				return nil
			}
			if observeIntent != nil {
				if err := observeQueuedIntent(observeIntent, intent); err != nil {
					return err
				}
			}
			if !orchestrator.beginner.Ready() {
				observeControlFlow(orchestrator.observer, ControlFlowBoundaryDroppedNotReady)
				continue
			}
			if orchestrator.gate != nil {
				decision, err := orchestrator.gate.AdmitBoundary(ctx, intent)
				if err != nil {
					if ctx.Err() != nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
						return ctx.Err()
					}
					observeControlFlow(orchestrator.observer, ControlFlowBoundaryAdmissionFailed)
					if errors.Is(err, ErrBoundaryInspectionUnavailable) && ctx.Err() == nil {
						continue
					}
					return err
				}
				if !decision.Valid() {
					observeControlFlow(orchestrator.observer, ControlFlowBoundaryAdmissionFailed)
					return errors.New("boundary admission gate returned an invalid decision")
				}
				switch decision {
				case BoundaryAdmissionBlockedLocalMenu:
					observeControlFlow(orchestrator.observer, ControlFlowBoundaryBlockedLocalMenu)
					continue
				case BoundaryAdmissionBlockedMouseCapture:
					observeControlFlow(orchestrator.observer, ControlFlowBoundaryBlockedMouseCapture)
					continue
				case BoundaryAdmissionBlockedPressedInput:
					observeControlFlow(orchestrator.observer, ControlFlowBoundaryBlockedPressedInput)
					continue
				case BoundaryAdmissionBlockedMoveSize:
					observeControlFlow(orchestrator.observer, ControlFlowBoundaryBlockedMoveSize)
					continue
				}
			}
			identity, err := orchestrator.identity()
			if err != nil {
				observeControlFlow(orchestrator.observer, ControlFlowBoundaryAdmissionFailed)
				return err
			}
			if err := orchestrator.beginner.BeginBoundary(ctx, intent, identity); err != nil {
				if errors.Is(err, ErrOutboundNotReady) {
					observeControlFlow(orchestrator.observer, ControlFlowBoundaryDroppedNotReady)
					continue
				}
				observeControlFlow(orchestrator.observer, ControlFlowBoundaryAdmissionFailed)
				return err
			}
			observeControlFlow(orchestrator.observer, ControlFlowBoundaryAdmitted)
		}
	}
}

func observeQueuedIntent(observer func(control.BoundaryIntent), intent control.BoundaryIntent) (err error) {
	defer func() {
		if recover() != nil {
			err = ErrBoundaryIntentObserverPanic
		}
	}()
	observer(intent)
	return nil
}

func NewRandomOutboundIdentity() (OutboundIdentity, error) {
	return randomOutboundIdentity(rand.Reader)
}

func randomOutboundIdentity(reader io.Reader) (OutboundIdentity, error) {
	if reader == nil {
		return OutboundIdentity{}, errors.New("random identity reader is required")
	}
	buffer := make([]byte, outboundIdentifierBytes*2)
	if _, err := io.ReadFull(reader, buffer); err != nil {
		return OutboundIdentity{}, err
	}
	return OutboundIdentity{
		SessionID: hex.EncodeToString(buffer[:outboundIdentifierBytes]),
		Epoch:     hex.EncodeToString(buffer[outboundIdentifierBytes:]),
	}, nil
}
