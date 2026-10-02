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
	var failure *connectionFailure
	if !errors.As(err, &failure) {
		return "installed_input_stage_unknown"
	}
	switch failure.stage {
	case "lease_metadata", "pipe_open", "service_pid", "service_manager", "service_open", "service_config", "service_status", "service_process_open", "service_process_metadata", "service_image", "service_image_acl", "lease_deadline", "lease_write", "worker_ready", "client_validation":
		return "installed_input_stage_" + failure.stage
	default:
		return "installed_input_stage_unknown"
	}
}

func ConnectionNativeFailureEvent(err error) string {
	var number syscall.Errno
	if !errors.As(err, &number) {
		return "installed_input_native_unknown"
	}
	switch number {
	case 2:
		return "installed_input_native_2"
	case 5:
		return "installed_input_native_5"
	case 32:
		return "installed_input_native_32"
	case 231:
		return "installed_input_native_231"
	case 1314:
		return "installed_input_native_1314"
	default:
		return "installed_input_native_other"
	}
}
