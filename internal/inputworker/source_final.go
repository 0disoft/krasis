package inputworker

import "errors"

// Called only after both native capture and sender results have been joined.
// An undelivered agent ACK is not missing native retirement proof: the service
// receives its own independently authenticated final worker report.
func sourceCaptureFinal(loop, capture, sender error, ownExpiry bool) error {
	result := errors.Join(loop, capture, sender)
	if sourceTransitionOnly(result, ownExpiry) {
		return nil
	}
	if !sourceTransitionOnly(capture, ownExpiry) {
		capture = &workerReportFailure{operation: "source-capture-join", cause: capture}
	}
	if !sourceTransitionOnly(sender, ownExpiry) {
		sender = &workerReportFailure{operation: "source-sender-join", cause: sender}
	}
	if !sourceTransitionOnly(loop, ownExpiry) {
		loop = &workerReportFailure{operation: "source-command-loop", cause: loop}
	}
	return errors.Join(loop, capture, sender, ErrRecovery)
}

func sourceRetirementDelivery(result, delivery error) error {
	if delivery == nil {
		return result
	}
	// result already contains ErrRecovery for any uncertain native/sender cleanup.
	// Failure to notify the agent must neither invent nor erase that uncertainty.
	return errors.Join(result, &workerReportFailure{operation: "source-retirement-write", cause: errors.Join(ErrProtocol, delivery)})
}
