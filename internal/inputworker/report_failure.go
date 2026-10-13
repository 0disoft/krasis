package inputworker

import (
	"errors"
	"syscall"

	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/wininput"
)

// Numeric, closed operation codes on the authenticated child/service channel.
// These are diagnostics only; status and release proof remain independent.
var reportOperations = [...]string{"", "low-level-hook", "raw-mouse", "wheel-capture", "thread-join", "hook-install", "hook-release", "raw-register", "raw-release", "raw-read", "raw-buffer", "raw-route", "message-loop", "pointer-snapshot", "cursor-present", "cursor-restore", "cursor-release"}

type workerReportFailure struct {
	operation string
	cause     error
}

func (e *workerReportFailure) Error() string                 { return "input worker failed" }
func (e *workerReportFailure) Unwrap() error                 { return e.cause }
func (e *workerReportFailure) LoginRuntimeOperation() string { return e.operation }

func reportFailureDetails(err error) (byte, uint32) {
	operation := wininput.SourceFailureStage(err)
	var code byte
	for index, name := range reportOperations {
		if name == operation {
			code = byte(index)
			break
		}
	}
	var native syscall.Errno
	if errors.As(err, &native) && native != 0 && uint64(native) <= 0xffffffff {
		return code, uint32(native)
	}
	return code, 0
}

func reportFailure(status, operation byte, native uint32) error {
	if status == 0 {
		return nil
	}
	cause := ErrAuthority
	if status == 2 {
		cause = ErrRecovery
	}
	if native != 0 {
		cause = errors.Join(cause, syscall.Errno(native))
	}
	if operation == 0 && native == 0 {
		return cause
	}
	return &workerReportFailure{operation: reportOperations[operation], cause: cause}
}

func validReportOperation(code byte) bool {
	return int(code) < len(reportOperations) && logindiag.ValidRuntimeOperation(reportOperations[code])
}
