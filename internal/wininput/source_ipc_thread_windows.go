//go:build windows

package wininput

import "runtime"

// StartSourceIPCReader starts one authenticated source-stream reader, not one
// thread per packet or lease crossing. A blocking native pipe read resumes on
// this dedicated thread instead of an arbitrary normal-priority Go thread.
// The priority hint is best effort; all results, deadlines and joins remain
// the caller's responsibility. Restore priority before returning the thread to
// the Go pool. If restoration fails, keep it locked through goroutine exit.
func StartSourceIPCReader(read func()) {
	go runSourceIPCReader(read)
}

func runSourceIPCReader(read func()) {
	runtime.LockOSThread()
	restore := sourceInputThreadPriorityScope(currentSourceThreadPriority, setSourceThreadPriority)
	defer func() {
		if restore() {
			runtime.UnlockOSThread()
		}
	}()
	read()
}
