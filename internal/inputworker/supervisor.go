package inputworker

import (
	"context"
	"errors"
	"net"
	"time"
)

type workerReportResult struct {
	report Report
	err    error
}

// supervise waits for the worker's recovery receipt, not just process exit.
// A missing/malformed receipt is recovery-unknown and stops the service from
// approving another lease. Forced job cleanup happens only after this returns.
// authorized is an owner-presence check, never an input authorization grant;
// native callers must not put SCM/WTS RPC on this cancellation/recovery loop.
func supervise(ctx context.Context, control net.Conn, authorized func() error, exited func() bool, expires time.Time) (Report, error) {
	results := make(chan workerReportResult, 1)
	done := make(chan struct{})
	go func() {
		defer close(done)
		report, err := readReport(control)
		results <- workerReportResult{report, err}
	}()
	defer func() { _ = control.Close(); <-done }()
	ticker := time.NewTicker(100 * time.Millisecond)
	defer ticker.Stop()
	return superviseWorkerResults(ctx, control, authorized, exited, expires, results, ticker.C)
}

func availableWorkerReceipt(results <-chan workerReportResult) (Report, error, bool) {
	select {
	case result := <-results:
		report, err := receipt(result.report, result.err)
		return report, err, true
	default:
		return Report{}, nil, false
	}
}

// Report completion is independent of optional policy polling. Prefer a fully
// published final result before entering synchronous broker queries or sending
// another stop. It still passes through receipt: an invalid or uncertain report
// never becomes success. The reader and its channel remain owned by supervise.
func superviseWorkerResults(ctx context.Context, control net.Conn, authorized func() error, exited func() bool, expires time.Time, results <-chan workerReportResult, polls <-chan time.Time) (Report, error) {
	for {
		if report, err, ready := availableWorkerReceipt(results); ready {
			return report, err
		}
		stop := false
		select {
		case result := <-results:
			return receipt(result.report, result.err)
		case <-ctx.Done():
			stop = true
		case <-polls:
			if report, err, ready := availableWorkerReceipt(results); ready {
				return report, err
			}
			stop = exited() || authorized() != nil || !time.Now().Before(expires)
		}
		if !stop {
			continue
		}
		// A report can arrive during the synchronous authority query. Consume
		// its exact outcome before a stop write/deadline affects the control pipe.
		if report, err, ready := availableWorkerReceipt(results); ready {
			return report, err
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
		operation := "worker-report-invalid"
		var failure *workerReportFailure
		if errors.As(err, &failure) && missingReportOperation(failure.operation) {
			operation = failure.operation
		}
		return Report{}, &workerReportFailure{operation: operation, cause: errors.Join(ErrRecovery, err)}
	}
	if report.Recovery.Remaining != 0 {
		return report, errors.Join(ErrRecovery, err)
	}
	return report, err
}
