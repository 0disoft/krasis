//go:build windows

package launcherui

import (
	"bytes"
	"os"
	"os/exec"
	"strconv"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

type clipboardForbiddenReader struct{ t *testing.T }

func (r clipboardForbiddenReader) Read([]byte) (int, error) {
	r.t.Fatal("helper parsed paths without an owned live parent")
	return 0, os.ErrClosed
}

func TestClipboardPackerRejectsMissingOrMalformedParentBeforeReading(t *testing.T) {
	for _, value := range []string{"", "0", "-1", "not-a-handle", "18446744073709551615"} {
		t.Setenv(clipboardPackParentEnvironment, value)
		var output bytes.Buffer
		if code := RunClipboardFilePacker(clipboardForbiddenReader{t}, &output); code != 2 || output.Len() != 0 {
			t.Fatal("invalid parent accepted", value, code)
		}
	}
}

func TestClipboardParentMonitorJoinsAndClosesAdoptedHandle(t *testing.T) {
	// No clipboard access or file I/O. Repeated adoption/stop must leave neither
	// the wait goroutine nor its inherited process reference alive.
	for range 16 {
		command := exec.Command("unused")
		command.SysProcAttr = &syscall.SysProcAttr{}
		closeParent, err := inheritClipboardPackParent(command)
		if err != nil {
			t.Fatal(err)
		}
		parent := windows.Handle(command.SysProcAttr.AdditionalInheritedHandles[0])
		var adopted windows.Handle
		if err := windows.DuplicateHandle(windows.CurrentProcess(), parent, windows.CurrentProcess(), &adopted, windows.SYNCHRONIZE, true, 0); err != nil {
			closeParent()
			t.Fatal(err)
		}
		closeParent()
		stop, err := watchClipboardPackParent(strconv.FormatUint(uint64(adopted), 10))
		if err != nil {
			t.Fatal(err)
		}
		stop()
		if _, err := windows.WaitForSingleObject(adopted, 0); err != windows.ERROR_INVALID_HANDLE {
			t.Fatal("monitor returned without closing its process reference", err)
		}
	}
}
