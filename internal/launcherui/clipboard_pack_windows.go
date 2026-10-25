//go:build windows

package launcherui

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"syscall"
	"time"

	"github.com/0disoft/krasis/internal/peerupdate"
)

const ClipboardPackArgument = "--clipboard-pack-internal"

// File traversal/open/read runs in a disposable child, so a blocked UNC/native
// call cannot own the UI's settings lock or shutdown lifetime. Paths travel on
// closed stdin, not the command line or logs. The child owns no product state.
func packClipboardFilesProcess(ctx context.Context, paths []string) ([]byte, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.CommandContext(ctx, self, ClipboardPackArgument)
	return runClipboardFilePacker(ctx, command, paths)
}

func runClipboardFilePacker(ctx context.Context, command *exec.Cmd, paths []string) ([]byte, error) {
	data, err := json.Marshal(paths)
	if err != nil {
		return nil, err
	}
	command.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: 0x08000000}
	closeParent, err := inheritClipboardPackParent(command)
	if err != nil {
		return nil, errClipboardUnavailable
	}
	defer closeParent()
	command.Stdin = bytes.NewReader(data)
	command.WaitDelay = time.Second
	var result clipboardPackBuffer
	command.Stdout = &result
	if err := command.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		var exit *exec.ExitError
		if errors.As(err, &exit) {
			switch exit.ExitCode() {
			case 3:
				return nil, errClipboardFileSize
			case 4:
				return nil, errClipboardFileCount
			}
		}
		return nil, errClipboardUnavailable
	}
	if !peerupdate.ValidClipboardFiles(result.Bytes()) {
		return nil, errClipboardUnavailable
	}
	return result.Bytes(), nil
}

type clipboardPackBuffer struct{ bytes.Buffer }

func (b *clipboardPackBuffer) Write(p []byte) (int, error) {
	if len(p) > peerupdate.MaxClipboardFileBytes-b.Len() {
		return 0, errClipboardFileSize
	}
	return b.Buffer.Write(p)
}

// An internal helper, not a resident background agent. Native I/O is isolated
// here; parent cancellation or death terminates this process even inside a
// blocked open. Only the parent's inherited wait-only bootstrap is accepted.
func RunClipboardFilePacker(input io.Reader, output io.Writer) int {
	stopParent, err := watchClipboardPackParent(os.Getenv(clipboardPackParentEnvironment))
	if err != nil {
		return 2
	}
	defer stopParent()
	var paths []string
	decoder := json.NewDecoder(io.LimitReader(input, 8<<20))
	if decoder.Decode(&paths) != nil {
		return 2
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return 2
	}
	data, err := packClipboardFiles(paths)
	if errors.Is(err, errClipboardFileSize) {
		return 3
	}
	if errors.Is(err, errClipboardFileCount) {
		return 4
	}
	if err != nil {
		return 2
	}
	if _, err := output.Write(data); err != nil {
		return 2
	}
	return 0
}
