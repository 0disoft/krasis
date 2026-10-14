//go:build windows

package inputworker

import (
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/logindiag"
	"golang.org/x/sys/windows"
)

func TestWorkerExitQueryUsesSignaledHandleBeforeReadingExitCode(t *testing.T) {
	for _, tc := range []struct {
		state            uint32
		waitErr, codeErr error
		code             uint32
		want             logindiag.WorkerExit
	}{
		{state: uint32(windows.WAIT_TIMEOUT), want: logindiag.WorkerExit{State: "RUNNING"}},
		{state: windows.WAIT_OBJECT_0, code: 259, want: logindiag.WorkerExit{State: "EXITED", Code: 259, Known: true}},
		{state: windows.WAIT_OBJECT_0, code: 0xc0000005, want: logindiag.WorkerExit{State: "EXITED", Code: 0xc0000005, Known: true}},
		{state: windows.WAIT_OBJECT_0, codeErr: errors.New("query failed"), want: logindiag.WorkerExit{State: "EXITED"}},
		{waitErr: errors.New("wait failed"), want: logindiag.WorkerExit{State: "UNAVAILABLE"}},
	} {
		queried := false
		got := inspectWorkerExit(func() (uint32, error) { return tc.state, tc.waitErr }, func() (uint32, error) { queried = true; return tc.code, tc.codeErr })
		if got != tc.want || queried != (tc.waitErr == nil && tc.state == windows.WAIT_OBJECT_0) {
			t.Fatal("liveness guessed from exit code or query failure", got)
		}
	}
	var zero ConsoleWorker
	if got := zero.reportExit(); got.State != "UNAVAILABLE" || zero.closed {
		t.Fatal("unstarted worker was stopped or classified as running", got)
	}
}
