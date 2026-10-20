package inputworker

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/logindiag"
)

func emptySourceExit() logindiag.SourceExit {
	return logindiag.SourceExit{Command: "not_started", Reader: "not_started", Capture: "not_started", Sender: "not_started", Interrupt: "not_started", Retirement: "not_sent", Report: "failed"}
}

func sourceStartupFailure(stage string, report Report, cause error) error {
	operation := "source-startup-" + stage
	if cause == nil || report.Source != nil || !logindiag.ValidRuntimeOperation(operation) {
		return cause
	}
	return &workerReportFailure{operation: operation, cause: cause}
}

func sourceCommandOutcome(err error) string {
	switch {
	case err == nil:
		return "ok"
	case sourceCleanupUncertain(err):
		return "cleanup"
	case sourceRetryableInspection(err):
		return "inspection"
	case errors.Is(err, ErrAuthority):
		return "authority"
	case errors.Is(err, ErrProtocol):
		return "protocol"
	case sourceTransitionOnly(err):
		return "cancelled"
	default:
		return "failed"
	}
}

func sourceReaderOutcome(err error) string {
	if err == nil {
		return "stopped"
	}
	if err == context.Canceled {
		return "cancelled"
	}
	switch sourceLoopOperation(err) {
	case "source-reader-eof":
		return "eof"
	case "source-reader-truncated":
		return "truncated"
	case "source-reader-timeout":
		return "timeout"
	case "source-reader-closed":
		return "closed"
	case "source-reader-invalid":
		return "invalid"
	case "source-reader-authority":
		return "authority"
	default:
		return "failed"
	}
}

func joinedSourceState(joined bool) string {
	if joined {
		return "joined"
	}
	return "unjoined"
}

func sourceRetirementOutcome(status byte, delivery error) string {
	if delivery != nil {
		return "delivery_failed"
	}
	if status != 0 {
		return "failed_sent"
	}
	return "clean_sent"
}

func validateSourceReportMode(capture bool, report Report, result error) error {
	if !capture && report.Source != nil {
		return errors.Join(result, ErrRecovery, &workerReportFailure{operation: "worker-report-invalid", cause: ErrProtocol})
	}
	return result
}
