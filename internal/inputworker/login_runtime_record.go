package inputworker

import (
	"github.com/0disoft/krasis/internal/logindiag"
	"sync"
	"time"
)

type loginRuntimeRecorder struct {
	mu      sync.Mutex
	value   logindiag.Runtime
	persist func(logindiag.Runtime) error
}

func newLoginRuntimeRecorder(persist func(logindiag.Runtime) error) *loginRuntimeRecorder {
	return &loginRuntimeRecorder{persist: persist, value: logindiag.Runtime{StartedAt: time.Now().UTC().Format(time.RFC3339Nano), Service: "STARTING", Console: "UNKNOWN", Grant: "UNKNOWN", LastWorker: "NONE"}}
}

// Writes only on decision transitions, not each 250ms policy observation.
// Reporting never authorizes, retries or suppresses an operation failure.
func (r *loginRuntimeRecorder) record(stage, value string, cause error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	next := r.value
	switch stage {
	case "console":
		next.Console = value
	case "grant":
		next.Grant = value
	case "start":
		next.LastWorker = value
	case "service":
		next.Service = value
	default:
		return
	}
	failure := logindiag.New("prepare", 3, cause)
	sameFailure := cause == nil || next.LastFailure != nil && next.LastFailure.Stage == stage && next.LastFailure.Win32Error == failure.Win32Error && next.LastFailure.Win32Known == failure.Win32Known
	if next == r.value && r.value.Time != "" && sameFailure {
		return
	}
	next.Time = time.Now().UTC().Format(time.RFC3339Nano)
	if cause != nil {
		next.LastFailure = &logindiag.RuntimeFailure{Time: next.Time, Stage: stage, Win32Error: failure.Win32Error, Win32Known: failure.Win32Known}
	}
	if !next.Valid() || r.persist == nil {
		return
	}
	if r.persist(next) == nil {
		r.value = next
	}
}
