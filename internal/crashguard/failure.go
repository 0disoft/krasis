package crashguard

import "errors"

var (
	ErrUnexpectedExit = errors.New("crash guard exited unexpectedly")
	ErrIPCTimeout     = errors.New("crash guard IPC timed out")
	ErrExitTimeout    = errors.New("crash guard exit timed out")
	ErrRecoveryFailed = errors.New("crash guard input recovery failed")
)

// ExitPending retains full retirement acknowledgement after Close's bounded
// wait: real child exit plus joined protocol and native cleanup operations.
// A timeout or child exit alone is not proof that ownership has been released.
type ExitPending struct {
	Done  <-chan struct{}
	Cause error
}

func (e *ExitPending) Error() string { return e.Cause.Error() }
func (e *ExitPending) Unwrap() error { return e.Cause }

type Failure struct {
	Operation string
	Cause     error
}

func (f *Failure) Error() string { return "crash guard " + f.Operation + ": " + f.Cause.Error() }
func (f *Failure) Unwrap() error { return f.Cause }
func operationFailure(operation string, err error) error {
	if err == nil {
		return nil
	}
	return &Failure{Operation: operation, Cause: err}
}
