package crashguard

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

const maxStderrBytes = 4096

// StartProduct uses only the recovery pipe protocol; it writes no probe report.
func StartProduct(executable string) (*Process, error) {
	if !filepath.IsAbs(executable) {
		return nil, errors.New("absolute guard executable required")
	}
	command := exec.Command(executable, "product-crash-guard-internal")
	command.Dir = filepath.Dir(executable)
	return startCommand(command)
}

var processIPCTimeout = 2 * time.Second

type Process struct {
	client  *Client
	stdin   io.WriteCloser
	stdout  io.ReadCloser
	process *os.Process
	done    chan struct{}
	abort   sync.Once

	mu      sync.Mutex
	waitErr error
	stderr  *limitedBuffer
}

func Start(executable, device, output string) (*Process, error) {
	if executable == "" || !filepath.IsAbs(executable) {
		return nil, errors.New("crash guard executable path must be absolute")
	}
	if device == "" || output == "" || !filepath.IsAbs(output) {
		return nil, errors.New("crash guard device and absolute output path are required")
	}
	command := exec.Command(executable, "crash-guard-internal", "--device", device, "--output", output)
	command.Dir = filepath.Dir(executable)
	return startCommand(command)
}

func startCommand(command *exec.Cmd) (*Process, error) {
	if command == nil {
		return nil, errors.New("crash guard command is required")
	}
	configureProcess(command)
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, fmt.Errorf("create crash guard input pipe: %w", err)
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("create crash guard output pipe: %w", err)
	}
	stderr := &limitedBuffer{remaining: maxStderrBytes}
	command.Stderr = stderr
	client, err := NewClient(stdin, stdout)
	if err != nil {
		_ = stdin.Close()
		return nil, err
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		return nil, fmt.Errorf("start crash guard: %w", err)
	}
	process := &Process{
		client: client, stdin: stdin, stdout: stdout, process: command.Process,
		done: make(chan struct{}), stderr: stderr,
	}
	go func() {
		waitErr := command.Wait()
		process.mu.Lock()
		process.waitErr = waitErr
		process.mu.Unlock()
		close(process.done)
	}()
	return process, nil
}

func (process *Process) PreparePress(token control.PressedToken) (bool, error) {
	if process == nil || process.client == nil {
		return false, errors.New("crash guard process is not initialized")
	}
	var prepared bool
	err := process.invoke(func() error {
		var err error
		prepared, err = process.client.PreparePress(token)
		return err
	})
	if err != nil {
		return false, err
	}
	return prepared, nil
}

func (process *Process) Released(token control.PressedToken) error {
	if process == nil || process.client == nil {
		return errors.New("crash guard process is not initialized")
	}
	return process.invoke(func() error { return process.client.Released(token) })
}

func (process *Process) CancelPreparedPress(token control.PressedToken) error {
	if process == nil || process.client == nil {
		return errors.New("crash guard process is not initialized")
	}
	return process.invoke(func() error { return process.client.CancelPreparedPress(token) })
}

func (process *Process) Done() <-chan struct{} {
	if process == nil {
		return nil
	}
	return process.done
}

func (process *Process) Close() error {
	if process == nil {
		return nil
	}
	disarmErr := process.invoke(process.client.Disarm)
	closeErr := process.stdin.Close()
	if errors.Is(closeErr, os.ErrClosed) {
		closeErr = nil
	}
	select {
	case <-process.done:
	case <-time.After(processIPCTimeout):
		process.abortProcess()
		<-process.done
		disarmErr = errors.Join(disarmErr, errors.New("crash guard exit timed out"))
	}
	process.mu.Lock()
	waitErr := process.waitErr
	stderr := process.stderr.String()
	process.mu.Unlock()
	if waitErr != nil && stderr != "" {
		waitErr = fmt.Errorf("crash guard exited: %w: %s", waitErr, stderr)
	}
	return errors.Join(disarmErr, closeErr, waitErr)
}

func (process *Process) invoke(operation func() error) error {
	result := make(chan error, 1)
	go func() { result <- operation() }()
	timer := time.NewTimer(processIPCTimeout)
	defer timer.Stop()
	select {
	case err := <-result:
		return err
	case <-timer.C:
		process.abortProcess()
		return errors.New("crash guard IPC timed out")
	}
}

func (process *Process) abortProcess() {
	process.abort.Do(func() {
		_ = process.stdin.Close()
		_ = process.stdout.Close()
		if process.process != nil {
			_ = process.process.Kill()
		}
	})
}

type limitedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	remaining int
}

func (buffer *limitedBuffer) Write(payload []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	written := len(payload)
	if buffer.remaining > 0 {
		keep := len(payload)
		if keep > buffer.remaining {
			keep = buffer.remaining
		}
		_, _ = buffer.buffer.Write(payload[:keep])
		buffer.remaining -= keep
	}
	return written, nil
}

func (buffer *limitedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return buffer.buffer.String()
}
