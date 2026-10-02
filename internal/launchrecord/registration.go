package launchrecord

import "errors"

// Registration failures expose only a fixed stage, never a path or raw OS error.
type registrationError struct {
	stage string
	cause error
}

func (e *registrationError) Error() string { return "launch registration failed: " + e.stage }
func (e *registrationError) Unwrap() error { return e.cause }

func registrationFailure(stage string, cause error) error {
	return &registrationError{stage: stage, cause: cause}
}

func RegistrationStage(err error) string {
	var failure *registrationError
	if errors.As(err, &failure) {
		switch failure.stage {
		case "image", "metadata", "directory", "cli", "script", "record":
			return failure.stage
		}
	}
	return "unknown"
}
