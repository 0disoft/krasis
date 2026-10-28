package inputworker

import (
	"errors"
	"net"
	"time"
)

// Called after native owners and release work have joined, before control closes.
func finishWorkerReport(control net.Conn, report Report, result error) error {
	// Readiness has its own deadline, which may be hours old. Final delivery
	// owns the existing independent one-second budget even after cancellation.
	if control.SetWriteDeadline(time.Now().Add(time.Second)) != nil {
		return errors.Join(result, ErrRecovery)
	}
	if writeReport(control, report, result) != nil {
		return errors.Join(result, ErrRecovery)
	}
	return result
}

// Unbound prepared children are not covered by a bound journal's recovery.
// Join their exact owners before publishing the final recovery receipt.
func finishInputWorkerReport(control net.Conn, report Report, result error, retire func() error) error {
	if retire != nil {
		if err := retire(); err != nil {
			result = errors.Join(result, err, ErrRecovery)
		}
	}
	return finishWorkerReport(control, report, result)
}
