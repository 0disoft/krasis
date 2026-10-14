package inputworker

import (
	"errors"

	"github.com/0disoft/krasis/internal/logindiag"
)

// This is an OS process result, never a Win32 operation error or recovery proof.
// The exact owned handle is inspected before job cleanup can terminate it.
type workerExitFailure struct {
	cause error
	exit  logindiag.WorkerExit
}

func (e *workerExitFailure) Error() string                                { return "worker report unavailable" }
func (e *workerExitFailure) Unwrap() error                                { return e.cause }
func (e *workerExitFailure) LoginRuntimeWorkerExit() logindiag.WorkerExit { return e.exit }

func observeMissingWorkerReport(result error, inspect func() logindiag.WorkerExit) error {
	var failure *workerReportFailure
	if inspect == nil || !errors.Is(result, ErrRecovery) || !errors.As(result, &failure) ||
		!missingReportOperation(failure.operation) {
		return result
	}
	exit := inspect()
	if !exit.Valid() {
		exit = logindiag.WorkerExit{State: "UNAVAILABLE"}
	}
	return &workerExitFailure{cause: result, exit: exit}
}

func missingReportOperation(operation string) bool {
	switch operation {
	case "worker-report-read", "worker-report-timeout", "worker-report-eof", "worker-report-truncated", "worker-report-invalid":
		return true
	}
	return false
}
