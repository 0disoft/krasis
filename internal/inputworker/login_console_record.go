package inputworker

import (
	"time"

	"github.com/0disoft/krasis/internal/logindiag"
)

type loginConsoleObservation struct {
	state   string
	failure *logindiag.RuntimeFailure
}

// The login gate serializes console observations. Publish only immutable
// diagnostics here: fresh WTS results still decide admission synchronously.
// The existing policy recorder and final service record persist the snapshot;
// neither disk I/O nor its mutex may delay input admission.
func (r *loginRuntimeRecorder) observeConsole(state string, cause error) {
	previous := r.console.Load()
	next := &loginConsoleObservation{state: state}
	if previous != nil {
		next.failure = previous.failure
		if cause == nil && previous.state == state {
			return
		}
	}
	if cause != nil {
		failure := logindiag.New("prepare", 3, cause)
		if previous != nil && previous.state == state && previous.failure != nil &&
			previous.failure.Win32Error == failure.Win32Error && previous.failure.Win32Known == failure.Win32Known {
			return
		}
		next.failure = &logindiag.RuntimeFailure{Time: time.Now().UTC().Format(time.RFC3339Nano), Stage: "console", Win32Error: failure.Win32Error, Win32Known: failure.Win32Known}
	}
	r.console.Store(next)
}
