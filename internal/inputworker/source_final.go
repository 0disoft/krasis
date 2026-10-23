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

// SourceSession.Activate only checks/updates Go session state. A rejection
// occurs before the separate native capture activation is attempted.
type sourceActivationRejected struct{ cause error }

func (e *sourceActivationRejected) Error() string { return "source session activation rejected" }
func (e *sourceActivationRejected) Unwrap() error { return e.cause }

func activateSourceSession(session *wininput.SourceSession) error {
	if err := session.Activate(); err != nil {
		return &sourceActivationRejected{cause: err}
	}
	return nil
}

func sourceTransportJoinedOnly(err error, ownExpiry bool, command bool) bool {
	if sourceTransitionOnly(err, ownExpiry) {
		return true
	}
	if sourceCleanupUncertain(err) {
		return false
	}
	if command && sourceReadQueryRejected(err) {
		return true
	}
	if _, ok := err.(*sourceTransportError); ok {
		return true
	}
	if _, ok := err.(*sourceActivationRejected); ok {
		return command
	}
	if inspection, ok := err.(*sourceInspectionRejected); ok {
		return command && sourceRetryableInspection(inspection.cause)
	}
	if failure, ok := err.(*authorityCheckFailure); ok {
		return command && validAuthorityOperation(failure.operation)
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
	progressRetired := wininput.SourceHookProgressRetiredOnly(capture)
	if !sourceTransitionOnly(capture, ownExpiry) {
		capture = &workerReportFailure{operation: "source-capture-join", cause: capture}
		uncertain = !progressRetired
	}
	if !sourceTransitionOnly(sender, ownExpiry) {
		uncertain = uncertain || !(sourceTransportJoinedOnly(sender, ownExpiry, false) || progressRetired && wininput.SourceHookProgressFailureOnly(sender))
		sender = &workerReportFailure{operation: "source-sender-join", cause: sender}
	}
	if !sourceTransitionOnly(loop, ownExpiry) {
		uncertain = uncertain || !sourceTransportJoinedOnly(loop, ownExpiry, true)
		loop = &workerReportFailure{operation: sourceLoopOperation(loop), cause: loop}
	}
	if uncertain {
		return errors.Join(loop, capture, sender, ErrRecovery)
	}
	return errors.Join(loop, capture, sender)
}

// All command, watchdog, native and sender callers must already be joined.
// A rejected read-only query cannot grant a clean retirement while a native
// query still owns either channel. Check before publishing status, not just
// after the agent's retirement notification has already been sent.
func sourceCaptureQueryRetirement(result error, owners ...*Channel) error {
	for _, owner := range owners {
		pending := owner == nil
		if owner != nil {
			owner.queryMu.Lock()
			pending = owner.pendingQueriesLocked()
			owner.queryMu.Unlock()
		}
		if pending {
			return errors.Join(result, ErrRecovery, &workerReportFailure{operation: "source-channel-close", cause: ErrAuthority})
		}
	}
	return result
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

func sourceFinalPacket(stopping *sourcePacket, result error, emergency bool) sourcePacket {
	status := sourceRetirementStatus(result)
	// A confirmed emergency remains terminal even if sourceStop arrived first
	// or a command was rejected earlier. Use the existing emergency packet;
	// never acknowledge it as an ordinary restartable shutdown. Uncertain
	// cleanup still takes priority and cannot claim clean retirement.
	if emergency && status == 0 {
		return sourcePacket{kind: sourceRetired, op: 1}
	}
	if stopping != nil {
		return sourcePacket{kind: sourceReply, op: sourceStop, reply: stopping.sequence, status: status}
	}
	return sourcePacket{kind: sourceRetired, status: status}
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
