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
	restore := sourceIPCReaderPriority(currentSourceThreadPriority, setSourceThreadPriority)
	defer func() {
		if restore() {
			runtime.UnlockOSThread()
		}
	}()
	read()
}

// False means that we changed the thread but cannot prove restoration. That
// thread must never be unlocked for unrelated Go work.
func sourceIPCReaderPriority(get func() int32, set func(int32) bool) func() bool {
	before := get()
	raised := promoteSourceInputThread(func() int32 { return before }, set)
	return func() bool { return !raised || set(before) }
}
