package inputworker

import (
	"errors"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
)

// Only the source writer may mark failure of its forwarding pipe. It owns no
// injection journal; a returned sender still has to join before this is used.
type sourceTransportError struct{ cause error }

func (e *sourceTransportError) Error() string { return "source transport failed" }
func (e *sourceTransportError) Unwrap() error { return e.cause }
func sourceTransportFailure(err error) error {
	if err == nil {
		return nil
	}
	return &sourceTransportError{cause: err}
}

func sourceTransportJoinedOnly(err error, ownExpiry bool, command bool) bool {
	if sourceTransitionOnly(err, ownExpiry) {
		return true
	}
	if sourceCleanupUncertain(err) {
		return false
	}
	if _, ok := err.(*sourceTransportError); ok {
		return true
	}
	if inspection, ok := err.(*sourceInspectionRejected); ok {
		return command && sourceRetryableInspection(inspection.cause)
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		for _, cause := range joined.Unwrap() {
			if !sourceTransportJoinedOnly(cause, ownExpiry, command) {
				return false
			}
		}
		return true
	}
	if wrapped, ok := err.(interface{ Unwrap() error }); ok {
		return sourceTransportJoinedOnly(wrapped.Unwrap(), ownExpiry, command)
	}
	// A rejected/failed command is not unknown native retirement. Preserve its
	// failure for the agent; the authenticated final report fences only if the
	// independent native/sender joins actually failed.
	return command && (err == ErrAuthority || err == ErrProtocol)
}

// Called only after both native capture and sender results have been joined.
// An undelivered agent ACK is not missing native retirement proof: the service
// receives its own independently authenticated final worker report.
func sourceCaptureFinal(loop, capture, sender error, ownExpiry bool) error {
	result := errors.Join(loop, capture, sender)
	if sourceTransitionOnly(result, ownExpiry) {
		return nil
	}
	uncertain := false
	if !sourceTransitionOnly(capture, ownExpiry) {
		capture = &workerReportFailure{operation: "source-capture-join", cause: capture}
		uncertain = true
	}
	if !sourceTransitionOnly(sender, ownExpiry) {
		uncertain = uncertain || !sourceTransportJoinedOnly(sender, ownExpiry, false)
		sender = &workerReportFailure{operation: "source-sender-join", cause: sender}
	}
	if !sourceTransitionOnly(loop, ownExpiry) {
		uncertain = uncertain || !sourceTransportJoinedOnly(loop, ownExpiry, true)
		loop = &workerReportFailure{operation: "source-command-loop", cause: loop}
	}
	if uncertain {
		return errors.Join(loop, capture, sender, ErrRecovery)
	}
	return errors.Join(loop, capture, sender)
}

func sourceRetirementDelivery(result, delivery error) error {
	if delivery == nil {
		return result
	}
	// result already contains ErrRecovery for any uncertain native/sender cleanup.
	// Failure to notify the agent must neither invent nor erase that uncertainty.
	return errors.Join(result, &workerReportFailure{operation: "source-retirement-write", cause: errors.Join(ErrProtocol, delivery)})
}

// Final source packets confirm retirement of native owners, not successful
// commands or new input authority. The separate service report retains the
// original command failure. Called only after sourceCaptureFinal joined both.
func sourceRetirementStatus(result error) byte {
	if sourceCleanupUncertain(result) {
		return 1
	}
	return 0
}

// Delivery failure cannot replace cleanup failure in the operation that was
// just executed. A safely rejected operation may continue after its reply.
func sourceCommandCompletion(operation, delivery error) error {
	if sourceCleanupUncertain(operation) {
		return errors.Join(ErrRecovery, operation, delivery)
	}
	if delivery != nil {
		return errors.Join(operation, delivery)
	}
	return nil // Caller separately retains safely rejected command evidence.
}

func sourceCleanupUncertain(err error) bool {
	return errors.Is(err, ErrRecovery) || errors.Is(err, inputdesktop.ErrCleanup) || errors.Is(err, wininput.ErrSourceCleanupIncomplete)
}
