package inputworker

// Read-only health failure with a fixed stage and preserved native cause.
// Stopped is set only after SCM successfully reports SERVICE_STOPPED.
type InstallationHealthError struct {
	Stage            string
	Stopped          bool
	RecoveryRequired bool
	Cause            error
}

func (e *InstallationHealthError) Error() string { return "protected input health verification failed" }
func (e *InstallationHealthError) Unwrap() error { return e.Cause }
