package agent

import (
	"context"
	"errors"
	"io"
	"net"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/cleanupresult"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
)

type recoveredCancellation struct {
	parent, cause error
}

func (e *recoveredCancellation) Error() string { return e.cause.Error() }
func (e *recoveredCancellation) Unwrap() error { return e.cause }

// RecoveredParentCancellation recognizes only the session owner's exact proof.
// Callers must inspect every joined sibling independently: an additional guard,
// transport or native cleanup error must still invalidate a clean shutdown.
func RecoveredParentCancellation(err, parent error) bool {
	proof, ok := err.(*recoveredCancellation)
	return ok && parent != nil && cleanupresult.OnlyCause(proof.parent, parent)
}

func recoveredParentCancellation(ctx context.Context, first error, result Result, cause error) error {
	r := result.Recovery
	parent, lifecycleStop := context.Cause(ctx).(*agentlifecycle.TransitionError)
	if !lifecycleStop || parent == nil || parent.Event.Validate() != nil ||
		ctx.Err() == nil || !cleanupresult.OnlyCause(first, parent) ||
		cause == nil || errors.Is(cause, ErrRecoveryUnverified) || !shutdownFailuresOnly(cause, context.Cause(ctx)) || result.FinalState != control.StateLocal ||
		r.DeadlineExceeded || r.ForwardingStopped != RecoveryConfirmed || r.WorkersJoined != RecoveryConfirmed ||
		r.NativeOwnersReleased != RecoveryConfirmed || r.PressedLedgerEmpty != RecoveryConfirmed {
		return cause
	}
	// Retain all original operation failures for diagnostics/errors.Is. They
	// are not cleanup failures when authoritative parent cancellation won first
	// and every input/native owner subsequently supplied independent proof.
	return &recoveredCancellation{parent: context.Cause(ctx), cause: cause}
}

func shutdownFailuresOnly(err, parent error) bool {
	if err == nil || err == parent { // parent is the validated transition pointer
		return true
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		children := joined.Unwrap()
		for _, child := range children {
			if child == nil || !shutdownFailuresOnly(child, parent) {
				return false
			}
		}
		return len(children) > 0
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		// Domain failure wrappers (release/effect/injection/control/write) are
		// not transparent evidence, even when their leaf is cancellation.
		_, component := err.(*sessionComponentError)
		if !component && !cleanupresult.Confirmed(err) {
			return false
		}
		child := wrapped.Unwrap()
		return child != nil && shutdownFailuresOnly(child, parent)
	}
	switch err {
	case context.Canceled, peer.ErrPeerInputDispatch, peer.ErrPeerRemoteEOF,
		peer.ErrPeerReadAfterLocalInterrupt, peer.ErrSourceSessionInterrupted,
		io.EOF, io.ErrUnexpectedEOF, io.ErrClosedPipe, net.ErrClosed:
		return true
	}
	return false
}
