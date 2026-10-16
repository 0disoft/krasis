package inputworker

import (
	"errors"
	"syscall"
)

// Connection failures expose fixed stages only. Paths, identities, pipe names
// and native error text never cross the diagnostic boundary.
type connectionFailure struct {
	stage string
	cause error
}

func (failure *connectionFailure) Error() string { return ErrAuthority.Error() }
func (failure *connectionFailure) Unwrap() []error {
	if failure.cause == nil {
		return []error{ErrAuthority}
	}
	return []error{ErrAuthority, failure.cause}
}

func connectionFailed(stage string, cause error) error {
	var existing *connectionFailure
	if errors.As(cause, &existing) {
		return cause
	}
	return &connectionFailure{stage: stage, cause: cause}
}

func ConnectionFailureEvent(err error) string {
	return connectionFailureEvent(err, "installed_input_stage_")
}

func connectionFailureEvent(err error, prefix string) string {
	var failure *connectionFailure
	if !errors.As(err, &failure) {
		return prefix + "unknown"
	}
	switch failure.stage {
	case "lease_metadata", "pipe_open", "service_pid", "service_manager", "service_open", "service_config", "service_status", "service_process_open", "service_process_metadata", "service_image", "service_image_acl", "lease_deadline", "lease_write", "worker_ready", "client_validation":
		return prefix + failure.stage
	default:
		return prefix + "unknown"
	}
}

func ConnectionNativeFailureEvent(err error) string {
	return connectionNativeFailureEvent(err, "installed_input_native_")
}

func connectionNativeFailureEvent(err error, prefix string) string {
	var number syscall.Errno
	if !errors.As(err, &number) {
		return prefix + "unknown"
	}
	switch number {
	case 2:
		return prefix + "2"
	case 5:
		return prefix + "5"
	case 32:
		return prefix + "32"
	case 231:
		return prefix + "231"
	case 1314:
		return prefix + "1314"
	default:
		return prefix + "other"
	}
}
