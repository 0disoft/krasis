package agent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

const (
	preparationHintLifetime = 500 * time.Millisecond
	// Keep margin beyond the independent two-second sender/receiver gates.
	// Renewal is driven only by fresh physical movement, never a timer.
	preparationHintRenewal = 2*time.Second + 100*time.Millisecond
)

type inputPreparer interface {
	PrepareInput(context.Context, transport.AuthenticatedPeerContext, time.Time)
}

// Observe runs on serialized physical input threads. Only enqueue a timestamp;
// network I/O and process creation never run under the hook's physical gate.
type preparationBoundary struct {
	next     wininput.BoundaryObserver
	config   control.BoundaryConfig
	hints    chan time.Time
	sent     bool
	sentAt   time.Time
	rearmAt  int64
	sampleX  int32
	sampleAt time.Time
	band     int64
}

func (p *preparationBoundary) Observe(action control.InputAction, x int32) error {
	if err := p.next.Observe(action, x); err != nil {
		return err
	}
	if action.Kind != control.InputPointerMove {
		return nil
	}
	p.offerPreparation(action, x, time.Now())
	return nil
}

func (p *preparationBoundary) offerPreparation(action control.InputAction, x int32, now time.Time) bool {
	if !p.preview(action, x, now) {
		return false
	}
	select {
	case p.hints <- now:
		p.preparationHintSent(now)
		return true
	default:
		// A full queue has not accepted this renewal. Fresh movement may retry
		// admission without consuming the local cooldown.
		return false
	}
}

func freshPreparationHint(at, now time.Time) bool {
	age := now.Sub(at)
	return !at.IsZero() && age >= 0 && age < preparationHintLifetime
}

func runPreparationSender(ctx context.Context, hints <-chan time.Time, runtime *application.SessionRuntime, coordinator *peer.SessionWriteCoordinator) error {
	var last time.Time
	payload, _ := transport.EncodeControlPayload(transport.ControlCommand{Kind: transport.ControlPrepareInput})
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case at := <-hints:
			now := time.Now()
			if !freshPreparationHint(at, now) || (!last.IsZero() && now.Sub(last) < 2*time.Second) {
				continue
			}
			last = now
			// Waiting for the semantic write gate is bounded. Hint expiry
			// before admission is harmless; an actual TLS write error is fatal.
			writeCtx, cancel := context.WithTimeout(ctx, preparationHintLifetime)
			attempted := false
			err := coordinator.Execute(writeCtx, func(writer peer.FrameWriter) error {
				if runtime.Snapshot().Control.State != control.StateLocal || !freshPreparationHint(at, time.Now()) {
					return nil
				}
				attempted = true
				return writer.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload})
			})
			cancel()
			if err != nil && attempted {
				// A frame writer failure may have partially written TLS data.
				// It cannot be treated as a harmless preparation rejection.
				return err
			}
		}
	}
}

func runPreparationReceiver(ctx context.Context, hints <-chan time.Time, runtime *application.SessionRuntime, preparer inputPreparer, authenticated transport.AuthenticatedPeerContext) error {
	var last time.Time
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case at := <-hints:
			now := time.Now()
			if preparer == nil || !freshPreparationHint(at, now) || (!last.IsZero() && now.Sub(last) < 2*time.Second) || runtime.Snapshot().Control.State != control.StateLocal {
				continue
			}
			last = now
			prepareCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
			preparer.PrepareInput(prepareCtx, authenticated, now)
			cancel()
		}
	}
}
