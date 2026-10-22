package inputworker

import (
	"errors"
	"syscall"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/wininput"
)

// Numeric, closed operation codes on the authenticated child/service channel.
// These are diagnostics only; status and release proof remain independent.
var reportOperations = []string{"", "low-level-hook", "raw-mouse", "wheel-capture", "thread-join", "hook-install", "hook-release", "raw-register", "raw-release", "raw-read", "raw-buffer", "raw-route", "message-loop", "pointer-snapshot", "cursor-present", "cursor-restore", "cursor-release", "raw-size", "raw-packet", "raw-desktop", "desktop-bind", "desktop-restore", "desktop-close", "desktop-thread-exit", "raw-window-create", "raw-window-destroy", "raw-ready", "desktop-current", "desktop-open", "desktop-name", "desktop-active", "source-capture-join", "source-sender-join", "source-command-loop", "source-retirement-write", "source-channel-close", "source-reader-read", "source-reader-eof", "source-reader-truncated", "source-reader-timeout", "source-reader-closed", "source-reader-invalid", "source-reader-authority"}

func init() {
	// Append only: existing operation bytes and the report layout stay stable.
	reportOperations = append(reportOperations, inputdesktop.CaptureThreadFailureOperations()...)
	reportOperations = append(reportOperations, "thread-join-low-level-hook", "thread-join-raw-mouse", "thread-join-wheel-capture", "thread-join-multiple")
	reportOperations = append(reportOperations, "authority-process-query", "authority-process-exited", "authority-console-session", "authority-console-token", "authority-logon-token", "authority-logon-changed", "authority-service-pipe", "authority-service-status")
	for op := sourceStart; op <= sourceCursorRestore; op++ {
		reportOperations = append(reportOperations, sourceCommandOperation(op))
	}
	reportOperations = append(reportOperations, "hook-callback-stalled", "hook-mouse-progress")
	reportOperations = append(reportOperations, "source-write-deadline", "source-write-unsubmitted", "source-write-partial", "source-write-timeout", "source-write-closed", "source-write-native", "source-write-other")
	reportOperations = append(reportOperations, sourceQueryFailureOperations()...)
	reportOperations = append(reportOperations, logindiag.SourceWriteOriginOperations()...)
	reportOperations = append(reportOperations, logindiag.SourceGateOperations()...)
	reportOperations = append(reportOperations, logindiag.SourceQueryDeadlineOperations()...)
}

type workerReportFailure struct {
	operation string
	cause     error
}

func (e *workerReportFailure) Error() string                 { return "input worker failed" }
func (e *workerReportFailure) Unwrap() error                 { return e.cause }
func (e *workerReportFailure) LoginRuntimeOperation() string { return e.operation }

func reportFailureDetails(err error) (byte, uint32) {
	operation, selected := wininput.SourceFailureDetails(err)
	if operation == "" {
		var failure interface {
			error
			LoginRuntimeOperation() string
		}
		if errors.As(err, &failure) && logindiag.ValidRuntimeOperation(failure.LoginRuntimeOperation()) {
			operation = failure.LoginRuntimeOperation()
			selected = failure
		}
	}
	var authority *authorityCheckFailure
	if errors.As(err, &authority) && validAuthorityOperation(authority.operation) {
		switch operation {
		case "", "low-level-hook", "raw-mouse", "wheel-capture", "source-reader-authority", "source-command-loop", "source-capture-join", "source-sender-join":
			// Refine generic containers, never a concrete native cleanup failure.
			operation, selected = authority.operation, authority
		default:
			if _, query := selected.(*sourceQueryFailure); query {
				operation, selected = authority.operation, authority
			}
		}
	}
	var query *sourceQueryFailure
	if errors.As(err, &query) {
		switch operation {
		case "", "source-reader-authority", "source-command-loop", "source-command-heartbeat", "source-sender-join":
			operation, selected = query.LoginRuntimeOperation(), query
		}
	}
	var command *sourceCommandFailure
	if errors.As(err, &command) {
		switch operation {
		case "", "source-command-loop":
			// Refine only the generic loop. Concrete native/authority failures
			// keep their own operation and the native code from that branch.
			operation, selected = sourceCommandOperation(command.op), command
		}
	}
	var code byte
	// Refine only the selected transport container. Searching all siblings here
	// would let a later failed retirement hide the original command/native cause.
	switch operation {
	case "", "source-command-loop", "source-sender-join", "source-command-heartbeat", "source-retirement-write":
		candidate := selected
		if candidate == nil {
			candidate = err
		}
		var write *sourceIOFailure
		var gate *sourceGateFailure
		if !sourceCleanupUncertain(err) && errors.As(candidate, &write) {
			operation, selected = write.operation(), write
		} else if !sourceCleanupUncertain(err) && errors.As(candidate, &gate) {
			operation, selected = gate.operation, gate
		}
	}
	for index, name := range reportOperations {
		if name == operation {
			code = byte(index)
			break
		}
	}
	if selected == authority && authority != nil {
		return code, authority.native
	}
	if selected == nil {
		selected = err
	}
	var native syscall.Errno
	if errors.As(selected, &native) && native != 0 && uint64(native) <= 0xffffffff {
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
