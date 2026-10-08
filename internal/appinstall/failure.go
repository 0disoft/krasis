package appinstall

import "errors"

// Keep the underlying error for ownership, cancellation and Win32 checks.
// Public diagnostics expose only the fixed operation name, never Error().
type installFailure struct {
	stage string
	cause error
}

func (e *installFailure) Error() string { return e.stage + ": " + e.cause.Error() }
func (e *installFailure) Unwrap() error { return e.cause }

func InstallationStage(err error) string {
	var failure *installFailure
	if errors.As(err, &failure) {
		return failure.stage
	}
	return ""
}

func installationFailure(stage string, err error) error {
	if err == nil || InstallationStage(err) != "" {
		return err
	}
	return &installFailure{stage, err}
}
