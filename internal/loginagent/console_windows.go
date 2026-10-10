//go:build windows

package loginagent

import (
	"unsafe"

	"golang.org/x/sys/windows"
)

var querySession = windows.NewLazySystemDLL("wtsapi32.dll").NewProc("WTSQuerySessionInformationW")

// ReadConsole is read-only. A failed query is never interpreted as an absent
// user. WTS retains the user name while locked, so lock/UAC cannot start a second
// connection owner. Names are inspected only for emptiness and never returned.
func ReadConsole() (Console, error) {
	return readConsole(windows.WTSGetActiveConsoleSessionId, readSession)
}

func readSession(session, class uint32) ([]byte, error) {
	var buffer *byte
	var size uint32
	ok, _, _ := querySession.Call(0, uintptr(session), uintptr(class), uintptr(unsafe.Pointer(&buffer)), uintptr(unsafe.Pointer(&size)))
	if buffer != nil {
		defer windows.WTSFreeMemory(uintptr(unsafe.Pointer(buffer)))
	}
	if ok == 0 || buffer == nil || size == 0 || size > 4096 {
		return nil, ErrObservation
	}
	return append([]byte(nil), unsafe.Slice(buffer, size)...), nil
}
