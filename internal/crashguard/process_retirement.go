package crashguard

import (
	"errors"
	"fmt"
	"os"
	"time"
)

func (process *Process) closeInputPipe() <-chan struct{} {
	process.stdinClose.Do(func() {
		process.stdinDone = make(chan struct{})
		go func() {
			err := process.stdin.Close()
			if errors.Is(err, os.ErrClosed) {
				err = nil
			}
			process.mu.Lock()
			process.stdinErr = err
			process.mu.Unlock()
			close(process.stdinDone)
		}()
	})
	return process.stdinDone
}

func (process *Process) closeOutputPipe() <-chan struct{} {
	process.stdoutClose.Do(func() {
		process.stdoutDone = make(chan struct{})
		go func() {
			err := process.stdout.Close()
			if errors.Is(err, os.ErrClosed) {
				err = nil
			}
			process.mu.Lock()
			process.stdoutErr = err
			process.mu.Unlock()
			close(process.stdoutDone)
		}()
	})
	return process.stdoutDone
}

// One retirement owner joins the real process exit, every native cleanup task
// and the sole protocol call. Only this complete boundary can unblock the next
// session through ExitPending.Done. Process.Done still reports actual exit to
// the live session monitor, independently of delayed native cleanup.
func (process *Process) closeOwned() {
	grace := time.Now().Add(processIPCTimeout)
	disarmErr := process.invokeOwned(process.client.Disarm, true)
	process.mu.Lock()
	process.closeCause = disarmErr
	process.mu.Unlock()
	process.closeInputPipe()
	timer := time.NewTimer(max(0, time.Until(grace)))
	select {
	case <-process.done:
	case <-timer.C:
		process.mu.Lock()
		process.closeCause = errors.Join(process.closeCause, ErrExitTimeout)
		process.mu.Unlock()
		process.requestAbort()
		<-process.done
	}
	timer.Stop()
	// Disarm admission has joined the protocol gate; closing now rejects all
	// later callers, so no Add can race this Wait.
	process.stopIPCExecutor()
	process.active.Wait()
	if process.executor != nil {
		<-process.executor.done
	}
	<-process.closeInputPipe()
	<-process.closeOutputPipe()
	if process.failed.Load() {
		<-process.requestAbort()
	}
	process.mu.Lock()
	waitErr := process.waitErr
	stderr := process.stderr.String()
	if waitErr != nil && stderr != "" {
		waitErr = fmt.Errorf("crash guard exited: %w: %s", waitErr, stderr)
	}
	process.closeErr = errors.Join(process.closeCause, process.stdinErr, process.stdoutErr, confirmedExitAbort(process.abortErr, process.exited), waitErr)
	process.mu.Unlock()
	close(process.closeDone)
}
