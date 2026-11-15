//go:build windows

package crashguard

import (
	"io"
	"os/exec"
)

// The guard/input ACK reader must park through Go's IOCP poller, rather than
// hold an OS thread in an anonymous-pipe ReadFile for every exchange. Only the
// private child write handle is inherited; framing and the shared buffered
// protocol reader remain unchanged. Close the parent's writer after Start,
// including failed Start, so child exit/abort can deliver EOF to that reader.
func configureStdout(command *exec.Cmd) (io.ReadCloser, func(), error) {
	reader, writer, err := newOverlappedGuardPipe("stdout")
	if err != nil {
		return nil, nil, err
	}
	command.Stdout = writer
	return reader, func() { _ = writer.Close() }, nil
}
