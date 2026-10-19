//go:build windows

package inputworker

import (
	"errors"

	"golang.org/x/sys/windows"
)

// A serial endpoint keeps its exclusive instance for the service lifetime.
// Each authenticated channel/worker gets a non-inheritable duplicate, so its
// close cannot unbind the name during cleanup, preparation or retry backoff.
// Only the endpoint loop calls borrow, after the previous worker has joined.
type servicePipeListener struct {
	pipe, loan *pipeConn
	closed     bool
	closeErr   error
}

func newServicePipeListener(name string, initial *pipeConn) (*servicePipeListener, error) {
	if initial == nil {
		var err error
		initial, err = makePipe(name)
		if err != nil {
			return nil, err
		}
	}
	return &servicePipeListener{pipe: initial}, nil
}

func (listener *servicePipeListener) borrow() (*pipeConn, error) {
	if listener == nil || listener.closed || listener.pipe == nil {
		return nil, servicePipeFailed("service-pipe-owner", nil)
	}
	if previous := listener.loan; previous != nil {
		select {
		case <-previous.closeDone:
		default:
			// Never disconnect a channel with an outstanding I/O owner.
			return nil, servicePipeFailed("service-pipe-owner", nil)
		}
		if previous.closeErr != nil {
			return nil, servicePipeFailed("service-pipe-close", previous.closeErr)
		}
		// All worker/native/report owners have joined before loop reentry.
		// Discard the retired client's unread bytes before authenticating a
		// new one. A client PID or a lease proof never survives this reset.
		if err := windows.DisconnectNamedPipe(listener.pipe.handle); err != nil && err != windows.ERROR_PIPE_NOT_CONNECTED {
			return nil, servicePipeFailed("service-pipe-reset", err)
		}
	}
	var handle windows.Handle
	if err := windows.DuplicateHandle(windows.CurrentProcess(), listener.pipe.handle, windows.CurrentProcess(), &handle, 0, false, windows.DUPLICATE_SAME_ACCESS); err != nil {
		return nil, servicePipeFailed("service-pipe-duplicate", err)
	}
	listener.loan = &pipeConn{handle: handle, closed: make(chan struct{}), closeDone: make(chan struct{})}
	return listener.loan, nil
}

func (listener *servicePipeListener) Close() error {
	if listener == nil {
		return nil
	}
	if listener.closed {
		return listener.closeErr
	}
	listener.closed = true
	if listener.loan != nil {
		listener.closeErr = listener.loan.Close()
	}
	if listener.pipe != nil {
		listener.closeErr = errors.Join(listener.closeErr, listener.pipe.Close())
	}
	if listener.closeErr != nil {
		listener.closeErr = servicePipeFailed("service-pipe-close", listener.closeErr)
	}
	return listener.closeErr
}

type servicePipeFailure struct {
	operation string
	cause     error
}

func (failure *servicePipeFailure) Error() string                 { return "input service listener failed" }
func (failure *servicePipeFailure) Unwrap() error                 { return failure.cause }
func (failure *servicePipeFailure) LoginRuntimeOperation() string { return failure.operation }
func servicePipeFailed(operation string, cause error) error {
	return &servicePipeFailure{operation: operation, cause: errors.Join(ErrRecovery, cause)}
}
