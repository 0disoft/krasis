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

type OutboundIdentity struct {
	SessionID string
	Epoch     string
}

type OutboundIdentityFactory func() (OutboundIdentity, error)

type BoundaryBeginner interface {
	Ready() bool
	BeginBoundary(context.Context, control.BoundaryIntent, OutboundIdentity) error
}

type BoundaryOrchestrator struct {
	beginner BoundaryBeginner
	identity OutboundIdentityFactory
}

func NewBoundaryOrchestrator(
	beginner BoundaryBeginner,
	identity OutboundIdentityFactory,
) (*BoundaryOrchestrator, error) {
	if beginner == nil || identity == nil {
		return nil, errors.New("boundary beginner and identity factory are required")
	}
	return &BoundaryOrchestrator{beginner: beginner, identity: identity}, nil
}

func (orchestrator *BoundaryOrchestrator) Run(
	ctx context.Context,
	intents <-chan control.BoundaryIntent,
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
			if !orchestrator.beginner.Ready() {
				continue
			}
			identity, err := orchestrator.identity()
			if err != nil {
				return err
			}
			if err := orchestrator.beginner.BeginBoundary(ctx, intent, identity); err != nil {
				if errors.Is(err, ErrOutboundNotReady) {
					continue
				}
				return err
			}
		}
	}
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
