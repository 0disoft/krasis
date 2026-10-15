package inputdesktop

// Diagnostic-only codes carry startup-thread equality and a closed wait
// outcome, never thread IDs, handles, desktop names or native exception text.
var captureThreadOperations = [...]string{
	"desktop-thread-pin-other", "desktop-thread-pin-startup",
	"desktop-thread-wait-other-timeout", "desktop-thread-wait-startup-timeout",
	"desktop-thread-wait-other-failed", "desktop-thread-wait-startup-failed",
	"desktop-thread-wait-other-unexpected", "desktop-thread-wait-startup-unexpected",
	"desktop-thread-close-other-signaled", "desktop-thread-close-startup-signaled",
	"desktop-thread-close-other-timeout", "desktop-thread-close-startup-timeout",
	"desktop-thread-close-other-failed", "desktop-thread-close-startup-failed",
	"desktop-thread-close-other-unexpected", "desktop-thread-close-startup-unexpected",
}

func CaptureThreadFailureOperations() []string {
	return append([]string(nil), captureThreadOperations[:]...)
}

func ValidCaptureThreadFailureOperation(operation string) bool {
	for _, allowed := range captureThreadOperations {
		if operation == allowed {
			return true
		}
	}
	return false
}

func captureThreadFailure(stage string, startup bool, wait string, cause error) error {
	owner := "other"
	if startup {
		owner = "startup"
	}
	operation := "desktop-thread-" + stage + "-" + owner
	if wait != "" {
		operation += "-" + wait
	}
	if !ValidCaptureThreadFailureOperation(operation) {
		operation = "desktop-thread-exit"
	}
	return operationFailure(operation, cause)
}
