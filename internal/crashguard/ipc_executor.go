package crashguard

import (
	"context"
	"sync"
)

// One protocol owner reuses one IPC goroutine and reply slot. callMu permits
// only one outstanding operation. A timed-out call permanently fences Process;
// its late reply cannot be consumed by another command or authorize a retry.
type ipcExecutor struct {
	work   chan ipcTask
	result chan error
	stop   chan struct{}
	done   chan struct{}
}

type ipcTask struct {
	ctx       context.Context
	operation func() error
	active    *sync.WaitGroup
}

func newIPCExecutor(exited <-chan struct{}) *ipcExecutor {
	e := &ipcExecutor{work: make(chan ipcTask), result: make(chan error, 1), stop: make(chan struct{}), done: make(chan struct{})}
	go func() {
		defer close(e.done)
		for {
			select {
			case <-e.stop:
				return
			case <-exited:
				return
			case work := <-e.work:
				err := work.ctx.Err()
				if err == nil {
					err = work.operation()
				}
				e.result <- err
				work.active.Done()
			}
		}
	}()
	return e
}

func (e *ipcExecutor) start(ctx context.Context, operation func() error, active *sync.WaitGroup) <-chan error {
	active.Add(1)
	select {
	case e.work <- (ipcTask{ctx, operation, active}):
	case <-e.done:
		active.Done()
		e.result <- ErrUnexpectedExit
	case <-ctx.Done():
		active.Done()
		e.result <- ctx.Err()
	}
	return e.result
}

// Shutdown has already fenced callers and joined callMu. Close the idle loop,
// then join any in-flight operation through the same retirement receipt.
func (process *Process) stopIPCExecutor() {
	process.callMu.Lock()
	if process.executor != nil {
		close(process.executor.stop)
	}
	process.callMu.Unlock()
}
