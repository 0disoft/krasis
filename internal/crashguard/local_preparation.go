package crashguard

import (
	"context"
	"errors"
	"io"
	"os/exec"
	"sync"
)

// LocalInputPreparation owns an unbound child waiting for its bootstrap. It
// exposes no input transport. Bind transfers ownership once; Close can only
// abort an unbound child and joins its exact exit and pipe cleanup.
type LocalInputPreparation struct {
	mu      sync.Mutex
	process *Process
	claimed bool
}

func PrepareLocalInputCommandContext(ctx context.Context, command *exec.Cmd) (*LocalInputPreparation, error) {
	return prepareLocalInputCommand(ctx, command, startCommand)
}

func prepareLocalInputCommand(ctx context.Context, command *exec.Cmd, start func(*exec.Cmd) (*Process, error)) (*LocalInputPreparation, error) {
	if ctx == nil || ctx.Err() != nil {
		return nil, ErrUnexpectedExit
	}
	p, err := start(command)
	if err != nil {
		return nil, err
	}
	p.portable = true
	prepared := &LocalInputPreparation{process: p}
	if err := ctx.Err(); err != nil {
		return nil, errors.Join(err, prepared.Close())
	}
	return prepared, nil
}

func (p *LocalInputPreparation) Bind(ctx context.Context, bootstrap []byte) (*Process, error) {
	if p == nil || ctx == nil || len(bootstrap) == 0 {
		return nil, ErrUnexpectedExit
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.claimed || p.process == nil {
		return nil, ErrUnexpectedExit
	}
	owner := p.process
	err := ctx.Err()
	if err == nil {
		err = owner.ExchangeInput(ctx, func(writer io.Writer, _ io.Reader) error {
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
		// Only bootstrap bytes could have been written; no journal or input
		// transport was published. Never disarm into a partial bootstrap.
		return nil, errors.Join(err, owner.closeUnbound())
	}
	p.claimed = true
	return owner, nil
}

func (p *LocalInputPreparation) Close() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.claimed || p.process == nil {
		return nil
	}
	return p.process.closeUnbound()
}

func (p *Process) closeUnbound() error {
	p.closeOnce.Do(func() {
		p.closing.Store(true)
		p.closeDone = make(chan struct{})
		go func() {
			<-p.requestAbort()
			<-p.done
			p.callMu.Lock()
			p.callMu.Unlock()
			p.active.Wait()
			<-p.closeInputPipe()
			<-p.closeOutputPipe()
			p.mu.Lock()
			// A killed, unbound child has no recovery obligation. Preserve
			// cleanup/native failures but do not classify its exit code as a
			// failed input recovery. Unknown exit still uses ExitPending.
			waitErr := unboundWaitFailure(p.waitErr)
			p.closeErr = errors.Join(waitErr, confirmedExitAbort(p.abortErr, p.exited), p.stdinErr, p.stdoutErr)
			p.mu.Unlock()
			close(p.closeDone)
		}()
	})
	return p.waitClose()
}

func unboundWaitFailure(err error) error {
	if _, ok := err.(*exec.ExitError); ok {
		return nil
	}
	if joined, ok := err.(interface{ Unwrap() []error }); ok {
		var failures []error
		for _, child := range joined.Unwrap() {
			failures = append(failures, unboundWaitFailure(child))
		}
		return errors.Join(failures...)
	}
	return err
}
