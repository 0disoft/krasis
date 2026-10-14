//go:build windows

package inputworker

import (
	"github.com/0disoft/krasis/internal/logindiag"
	"golang.org/x/sys/windows"
)

func (worker *ConsoleWorker) reportExit() logindiag.WorkerExit {
	if worker == nil {
		return logindiag.WorkerExit{State: "UNAVAILABLE"}
	}
	worker.mu.Lock()
	defer worker.mu.Unlock()
	if worker.closed || worker.process == 0 {
		return logindiag.WorkerExit{State: "UNAVAILABLE"}
	}
	return inspectWorkerExit(func() (uint32, error) { return windows.WaitForSingleObject(worker.process, 0) }, func() (uint32, error) {
		var code uint32
		err := windows.GetExitCodeProcess(worker.process, &code)
		return code, err
	})
}

// An exit code of 259 is legitimate after a signaled process; never infer
// process liveness from STILL_ACTIVE alone. Queries do not stop or wait for it.
func inspectWorkerExit(wait func() (uint32, error), code func() (uint32, error)) logindiag.WorkerExit {
	state, err := wait()
	if err != nil {
		return logindiag.WorkerExit{State: "UNAVAILABLE"}
	}
	if state == uint32(windows.WAIT_TIMEOUT) {
		return logindiag.WorkerExit{State: "RUNNING"}
	}
	if state != windows.WAIT_OBJECT_0 {
		return logindiag.WorkerExit{State: "UNAVAILABLE"}
	}
	exit, err := code()
	if err != nil {
		return logindiag.WorkerExit{State: "EXITED"}
	}
	return logindiag.WorkerExit{State: "EXITED", Code: exit, Known: true}
}
