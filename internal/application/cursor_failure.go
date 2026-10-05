package application

// CursorTransactionFailure identifies a local transaction stage, not the native
// cause or an input outcome. The original error remains available to recovery.
type CursorTransactionFailure struct {
	Operation string
	Stage     string
	Cause     error
}

func (*CursorTransactionFailure) Error() string { return "cursor transaction failed" }

func (failure *CursorTransactionFailure) Unwrap() error {
	if failure == nil {
		return nil
	}
	return failure.Cause
}

// ValidCursorFailureStage is also the export allowlist. No error text, position
// or target identity is accepted as a diagnostic stage.
func ValidCursorFailureStage(operation, stage string) bool {
	switch operation {
	case "entry":
		switch stage {
		case "context", "state", "snapshot", "geometry", "set", "verify", "mismatch":
			return true
		}
	case "rollback":
		switch stage {
		case "context", "state", "unverified", "inspect", "restore", "verify", "mismatch":
			return true
		}
	}
	return false
}
