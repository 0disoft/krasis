package crashguard

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/syncgate"
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

// StartPortable starts the fixed ordinary input child. Product callers use
// StartPortableProcesses to retain an independent guardian and full retirement.
// It never constructs a privileged service proof.
func StartPortable(executable string) (*Process, error) {
	if !filepath.IsAbs(executable) {
		return nil, errors.New("absolute guard executable required")
	}
	command := exec.Command(executable, "product-portable-input-internal")
	command.Dir = filepath.Dir(executable)
	process, err := startCommand(command)
	if err == nil {
		process.portable = true
	}
	return process, err
}

// StartLocalInputCommand owns only local child transport and retirement. A
// privileged dispatcher must independently authenticate its bootstrap before
// accepting input; this constructor supplies no Windows/service authority.
func StartLocalInputCommand(command *exec.Cmd, bootstrap []byte) (*Process, error) {
	return StartLocalInputCommandContext(context.Background(), command, bootstrap)
}

// StartLocalInputCommandContext shares the startup caller's remaining budget.
// Cancellation retires the exact launched child; it never grants input authority.
func StartLocalInputCommandContext(ctx context.Context, command *exec.Cmd, bootstrap []byte) (*Process, error) {
	return startLocalInputCommand(ctx, command, bootstrap, startCommand)
}

func startLocalInputCommand(ctx context.Context, command *exec.Cmd, bootstrap []byte, start func(*exec.Cmd) (*Process, error)) (*Process, error) {
	if ctx == nil {
		return nil, ErrUnexpectedExit
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	process, err := start(command)
	if err != nil {
		return nil, err
	}
	process.portable = true
	// Native process creation can finish after cancellation. Retire that exact
	// owner before returning; it must not receive a late bootstrap or input.
	err = ctx.Err()
	if err == nil && len(bootstrap) != 0 {
		err = process.ExchangeInput(ctx, func(writer io.Writer, _ io.Reader) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			n, err := writer.Write(bootstrap)
			if err != nil || n != len(bootstrap) {
				return errors.Join(ErrUnexpectedExit, err)
			}
			return nil
		})
	}
	if err == nil {
		err = ctx.Err()
	}
	if err != nil {
		// No input journal exists during startup. Abort instead of sending a
		// graceful disarm into a child that may still be reading its bootstrap.
		process.requestAbort()
		return process, errors.Join(err, process.Close())
	}
	return process, nil
}

var processIPCTimeout = 2 * time.Second

type Process struct {
	client      *Client
	stdin       io.WriteCloser
	stdout      io.ReadCloser
	process     *os.Process
	done        chan struct{}
	abort       sync.Once
	abortDone   chan struct{}
	abortErr    error
	kill        func() error
	callMu      syncgate.Mutex
	failed      atomic.Bool
	closing     atomic.Bool
	active      sync.WaitGroup
	executor    *ipcExecutor // Created and stopped only while callMu is held.
	closeOnce   sync.Once
	closeDone   chan struct{}
	closeErr    error
	closeCause  error
	stdinClose  sync.Once
	stdinDone   chan struct{}
	stdinErr    error
	stdoutClose sync.Once
	stdoutDone  chan struct{}
	stdoutErr   error
	portable    bool
	// Borrowed from PortableProcesses; that owner outlives both children.
	portableOwnership uintptr

	mu      sync.Mutex
	waitErr error
	exited  bool
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
	started, finishStderr, err := configureStderr(command, stderr)
	if err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		return nil, fmt.Errorf("create crash guard error pipe: %w", err)
	}
	client, err := NewClient(stdin, stdout)
	if err != nil {
		_ = stdin.Close()
		started()
		_ = finishStderr()
		return nil, err
	}
	if err := command.Start(); err != nil {
		_ = stdin.Close()
		_ = stdout.Close()
		started()
		_ = finishStderr()
		return nil, fmt.Errorf("start crash guard: %w", err)
	}
	started()
	process := &Process{
		client: client, stdin: stdin, stdout: stdout, process: command.Process,
		done: make(chan struct{}), stderr: stderr,
	}
	go func() {
		waitErr := errors.Join(waitProcess(command), finishStderr())
		process.mu.Lock()
		process.waitErr = waitErr
		process.exited = command.ProcessState != nil
		process.mu.Unlock()
		close(process.done)
	}()
	return process, nil
}

func (process *Process) PreparePress(token control.PressedToken) (bool, error) {
	return process.PreparePressContext(context.Background(), token)
}

// Preparation precedes injection and shares its caller's remaining budget.
// An admitted timeout still poisons and retires the exact protocol owner.
func (process *Process) PreparePressContext(ctx context.Context, token control.PressedToken) (bool, error) {
	if process == nil || process.client == nil || ctx == nil {
		return false, errors.New("crash guard process is not initialized")
	}
	var prepared bool
	err := process.invokeContext(ctx, func() error {
		var err error
		prepared, err = process.client.PreparePress(token)
		return err
	}, false)
	if err != nil {
		return false, operationFailure("prepare", err)
	}
	return prepared, nil
}

func (process *Process) Released(token control.PressedToken) error {
	if process == nil || process.client == nil {
		return errors.New("crash guard process is not initialized")
	}
	return operationFailure("release", process.invoke(func() error { return process.client.Released(token) }))
}

func (process *Process) CancelPreparedPress(token control.PressedToken) error {
	if process == nil || process.client == nil {
		return errors.New("crash guard process is not initialized")
	}
	return operationFailure("cancel-prepare", process.invoke(func() error { return process.client.CancelPreparedPress(token) }))
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
	process.closeOnce.Do(func() {
		process.closing.Store(true)
		process.closeDone = make(chan struct{})
		go process.closeOwned()
	})
	return process.waitClose()
}

func (process *Process) waitClose() error {
	timer := time.NewTimer(2 * processIPCTimeout)
	defer timer.Stop()
	select {
	case <-process.closeDone:
		return operationFailure("close", process.closeErr)
	case <-timer.C:
		select {
		case <-process.closeDone:
			return operationFailure("close", process.closeErr)
		default:
		}
		process.mu.Lock()
		cause := errors.Join(ErrExitTimeout, process.closeCause, process.abortErr, process.stdinErr, process.stdoutErr)
		process.mu.Unlock()
		return operationFailure("close", &ExitPending{Done: process.closeDone, Cause: cause})
	}
}

func (process *Process) invoke(operation func() error) error {
	return process.invokeOwned(operation, false)
}

// ExchangeInput serializes one whole input frame/reply with guard ledger IPC.
// A caller cannot supply another process, pipe, executable or desktop.
func (process *Process) ExchangeInput(ctx context.Context, exchange func(io.Writer, io.Reader) error) error {
	if process == nil || process.client == nil || !process.portable || ctx == nil || exchange == nil {
		return ErrUnexpectedExit
	}
	// invokeContext holds callMu across ledger and input exchanges. Bypassing
	// this shared reader would lose bytes prefetched by a preceding guard ACK.
	return process.invokeContext(ctx, func() error { return exchange(process.stdin, process.client.reader) }, false)
}

func (process *Process) invokeOwned(operation func() error, shutdown bool) error {
	return process.invokeContext(context.Background(), operation, shutdown)
}

func (process *Process) invokeContext(parent context.Context, operation func() error, shutdown bool) error {
	ctx, cancel := context.WithTimeout(parent, processIPCTimeout)
	defer cancel()
	// Waiting callers cannot create a goroutine behind the protocol mutex.
	// Admission consumes the same budget as the exchange; it grants no IPC.
	if err := process.callMu.LockContext(ctx); err != nil {
		return errors.Join(ErrIPCTimeout, err)
	}
	defer process.callMu.Unlock()
	if process.failed.Load() || process.closing.Load() && !shutdown {
		return ErrIPCTimeout
	}
	var result <-chan error
	if shutdown {
		// Disarm must still read retained protocol failure/ACK evidence after
		// process exit. It runs once and is joined by retirement as before.
		reply := make(chan error, 1)
		process.active.Add(1)
		go func() { defer process.active.Done(); reply <- operation() }()
		result = reply
	} else {
		if process.executor == nil {
			process.executor = newIPCExecutor(process.done)
		}
		result = process.executor.start(ctx, operation, &process.active)
	}
	select {
	case err := <-result:
		if ctx.Err() == nil && !process.failed.Load() && (!process.closing.Load() || shutdown) {
			return err
		}
	case <-ctx.Done():
	}
	process.requestAbort()
	return errors.Join(ErrIPCTimeout, ctx.Err())
}

// Abort/pipe-close calls have their own single owners. A native call that does
// not return cannot hold the caller's input/shutdown gate or spawn replacements.
func (process *Process) requestAbort() <-chan struct{} {
	process.failed.Store(true)
	process.abort.Do(func() {
		process.abortDone = make(chan struct{})
		go func() {
			var err error
			if process.kill != nil {
				err = process.kill()
			} else if process.process != nil {
				err = process.process.Kill()
			}
			if errors.Is(err, os.ErrProcessDone) {
				err = nil
			}
			process.mu.Lock()
			process.abortErr = err
			process.mu.Unlock()
			close(process.abortDone)
		}()
		process.closeInputPipe()
		process.closeOutputPipe()
	})
	return process.abortDone
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
