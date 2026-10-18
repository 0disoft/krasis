//go:build windows

package crashguard

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"os/exec"
	"unsafe"

	"golang.org/x/sys/windows"
)

// A writer-valued Cmd.Stderr makes os/exec read a synchronous anonymous pipe
// forever. Give the child a file handle instead, and drain an overlapped read
// handle through Go's IOCP poller. The same 4 KiB memory limit still applies;
// there is no persistent stderr file or new diagnostic content.
func configureStderr(command *exec.Cmd, sink *limitedBuffer) (func(), func() error, error) {
	reader, writer, err := overlappedGuardPipe()
	if err != nil {
		return nil, nil, err
	}
	command.Stderr = writer
	done := make(chan error, 1)
	go func() { _, err := io.Copy(sink, reader); done <- err }()
	return func() { _ = writer.Close() }, func() error {
		return errors.Join(<-done, reader.Close())
	}, nil
}

func overlappedGuardPipe() (*os.File, *os.File, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, nil, err
	}
	descriptor, err := windows.SecurityDescriptorFromString("D:P(A;;GA;;;" + user.User.Sid.String() + ")(A;;GA;;;SY)")
	if err != nil {
		return nil, nil, err
	}
	var nonce [16]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, nil, err
	}
	name, err := windows.UTF16PtrFromString(`\\.\pipe\Krasis.Guard.Stderr.` + hex.EncodeToString(nonce[:]))
	if err != nil {
		return nil, nil, err
	}
	security := windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: descriptor}
	readHandle, err := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_INBOUND|windows.FILE_FLAG_OVERLAPPED|windows.FILE_FLAG_FIRST_PIPE_INSTANCE,
		windows.PIPE_TYPE_BYTE|windows.PIPE_READMODE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS, 1, 4096, 4096, 0, &security)
	if err != nil {
		return nil, nil, err
	}
	writeHandle, err := windows.CreateFile(name, windows.GENERIC_WRITE, 0, nil, windows.OPEN_EXISTING, windows.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		_ = windows.CloseHandle(readHandle)
		return nil, nil, err
	}
	// The private client was opened above, so there is no pending connect IO
	// whose OVERLAPPED could outlive this stack frame.
	var overlap windows.Overlapped
	if err := windows.ConnectNamedPipe(readHandle, &overlap); err != nil && err != windows.ERROR_PIPE_CONNECTED {
		_ = windows.CloseHandle(writeHandle)
		_ = windows.CloseHandle(readHandle)
		return nil, nil, err
	}
	return os.NewFile(uintptr(readHandle), "guard-stderr-read"), os.NewFile(uintptr(writeHandle), "guard-stderr-write"), nil
}
