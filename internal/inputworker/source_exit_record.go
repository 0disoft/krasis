package inputworker

import (
	"errors"
	"math"
	"time"

	"github.com/0disoft/krasis/internal/logindiag"
)

const maxLoginRuntimeBytes = 16 << 10

// Uses the service's clock, not the child/peer clock. Publishing never takes the
// persistence mutex or performs disk I/O on the source endpoint. The existing
// policy recorder (or final service record) persists the immutable history.
func (r *loginRuntimeRecorder) recordSourceExit(report Report, result error) {
	if report.Source == nil || !report.Source.ValidDetails() || report.Source.Time != "" || report.Source.Failure != nil {
		return
	}
	exit := *report.Source
	exit.Time = time.Now().UTC().Format(time.RFC3339Nano)
	if result != nil {
		op, native := reportFailureDetails(result)
		operation := reportOperations[op]
		if operation == "" {
			var detail interface{ LoginRuntimeOperation() string }
			if errors.As(result, &detail) && logindiag.ValidRuntimeOperation(detail.LoginRuntimeOperation()) {
				operation = detail.LoginRuntimeOperation()
			}
		}
		exit.Failure = &logindiag.SourceFailure{Operation: operation, Win32Error: native, Win32Known: native != 0}
	}
	// The serial source endpoint is the only publisher. Policy observations
	// only load this pointer, so publication needs neither a retry loop nor I/O.
	previous := r.source.Load()
	history := &logindiag.SourceHistory{}
	if previous != nil {
		history.Total = previous.Total
		history.Exits = append(history.Exits, previous.Exits...)
	}
	if history.Total == math.MaxUint64 {
		return
	}
	history.Total++
	history.Exits = append(history.Exits, exit)
	if len(history.Exits) > logindiag.MaxSourceExits {
		history.Exits = append([]logindiag.SourceExit(nil), history.Exits[len(history.Exits)-logindiag.MaxSourceExits:]...)
	}
	r.source.Store(history)
}
