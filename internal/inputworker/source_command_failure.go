package inputworker

// Retain the command that failed independently of later cancellation, reply
// delivery, and retirement. This marker changes diagnostics only, not cleanup
// classification, and never retains command payloads or arbitrary error text.
type sourceCommandFailure struct {
	op    byte
	cause error
}

func (e *sourceCommandFailure) Error() string { return "source command failed" }
func (e *sourceCommandFailure) Unwrap() error { return e.cause }

func sourceCommandFailed(op byte, err error) error {
	if err == nil {
		return nil
	}
	return &sourceCommandFailure{op: op, cause: err}
}

func sourceCommandOperation(op byte) string {
	switch op {
	case sourceStart:
		return "source-command-start"
	case sourceAdmit:
		return "source-command-admit"
	case sourceActivate:
		return "source-command-activate"
	case sourceDeactivate:
		return "source-command-deactivate"
	case sourceHeartbeat:
		return "source-command-heartbeat"
	case sourceStop:
		return "source-command-stop"
	case sourceCursorPrepare:
		return "source-command-cursor-prepare"
	case sourceCursorShow:
		return "source-command-cursor-show"
	case sourceCursorRestore:
		return "source-command-cursor-restore"
	}
	return "source-command-loop"
}
