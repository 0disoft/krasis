package inputworker

import (
	"errors"
	"syscall"
)

// Only fixed operations and a numeric Win32 result survive. In particular this
// does not retain an OS error wrapper, token, process, identity or error text.
type authorityCheckFailure struct {
	operation string
	native    uint32
}

func (f *authorityCheckFailure) Error() string { return ErrAuthority.Error() }
func (f *authorityCheckFailure) Unwrap() []error {
	if f.native != 0 {
		return []error{ErrAuthority, syscall.Errno(f.native)}
	}
	return []error{ErrAuthority}
}
func (f *authorityCheckFailure) LoginRuntimeOperation() string { return f.operation }

func authorityCheckFailed(operation string, cause error) error {
	if !validAuthorityOperation(operation) {
		return ErrAuthority
	}
	var native syscall.Errno
	var number uint32
	if errors.As(cause, &native) && native != 0 && uint64(native) <= 0xffffffff {
		number = uint32(native)
	}
	return &authorityCheckFailure{operation: operation, native: number}
}

func validAuthorityOperation(operation string) bool {
	switch operation {
	case "authority-process-query", "authority-process-exited", "authority-console-session", "authority-console-token", "authority-logon-token", "authority-logon-changed", "authority-service-pipe", "authority-service-status":
		return true
	}
	return false
}

// This projection is used only inside source workers. An arbitrary callback
// error cannot supply diagnostic labels or retain raw error details.
func sourceAuthorityResult(cause error) error {
	if cause == nil {
		return nil
	}
	var failure *authorityCheckFailure
	if errors.As(cause, &failure) {
		return authorityCheckFailed(failure.operation, failure)
	}
	var connection *connectionFailure
	if errors.As(cause, &connection) {
		switch connection.stage {
		case "service_pid":
			return authorityCheckFailed("authority-service-pipe", connection.cause)
		case "service_status":
			return authorityCheckFailed("authority-service-status", connection.cause)
		}
	}
	return ErrAuthority
}
