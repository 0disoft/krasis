package inputworker

import (
	"context"
	"errors"
	"net"
	"time"
)

// supervise waits for the worker's recovery receipt, not just process exit.
// A missing/malformed receipt is recovery-unknown and stops the service from
// approving another lease. Forced job cleanup happens only after this returns.
func supervise(ctx context.Context, control net.Conn, authorized func() error, exited func() bool, expires time.Time) (Report, error) {
	type result struct {
		report Report
		err    error
	}
	results := make(chan result, 1)
	done := make(chan struct{})
	go func() { defer close(done); report, err := readReport(control); results <- result{report, err} }()
	defer func() { _ = control.Close(); <-done }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	for {
		stop := false
		select {
		case result := <-results:
			return receipt(result.report, result.err)
		case <-ctx.Done():
			stop = true
		case <-ticker.C:
			stop = exited() || authorized() != nil || !time.Now().Before(expires)
		}
		if !stop {
			continue
		}
		if !exited() {
			_ = control.SetWriteDeadline(time.Now().Add(250 * time.Millisecond))
			_ = writeFrame(control, []byte{'S'})
		}
		_ = control.SetReadDeadline(time.Now().Add(RecoveryTimeout + time.Second))
		timer := time.NewTimer(RecoveryTimeout + time.Second)
		defer timer.Stop()
		select {
		case result := <-results:
			return receipt(result.report, result.err)
		case <-timer.C:
			return Report{}, &workerReportFailure{operation: "worker-report-timeout", cause: ErrRecovery}
		}
	}
}

func receipt(report Report, err error) (Report, error) {
	if errors.Is(err, ErrProtocol) {
		return Report{}, &workerReportFailure{operation: "worker-report-read", cause: errors.Join(ErrRecovery, err)}
	}
	if report.Recovery.Remaining != 0 {
		return report, errors.Join(ErrRecovery, err)
	}
	return report, err
}
