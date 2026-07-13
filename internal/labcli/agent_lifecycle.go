package labcli

import (
	"context"
	"errors"
	"sync"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/winlifecycle"
)

var errCrashGuardStopped = errors.New("crash guard stopped unexpectedly")

type ownedCrashGuard interface {
	PreparePress(control.PressedToken) error
	Released(control.PressedToken) error
	Done() <-chan struct{}
	Close() error
}

type agentSessionProcessOwner struct {
	osOwner *agentlifecycle.Owner
	context context.Context
	cancel  context.CancelCauseFunc

	mu     sync.Mutex
	source agent.ConnectionSource
	guard  ownedCrashGuard

	guardWatcherDone chan struct{}
	closing          bool
	close            sync.Once
	closeResult      agentSessionProcessCloseResult
}

type agentSessionProcessCloseResult struct {
	SourceErr    error
	GuardErr     error
	LifecycleErr error
}

func startAgentSessionProcessOwner(parent context.Context) (*agentSessionProcessOwner, error) {
	return newAgentSessionProcessOwner(parent, winlifecycle.StartMonitor)
}

func newAgentSessionProcessOwner(parent context.Context, start agentlifecycle.StartMonitor) (*agentSessionProcessOwner, error) {
	osOwner, err := agentlifecycle.Start(parent, start)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancelCause(osOwner.Context())
	return &agentSessionProcessOwner{osOwner: osOwner, context: ctx, cancel: cancel}, nil
}

func (owner *agentSessionProcessOwner) Context() context.Context {
	if owner == nil {
		return nil
	}
	return owner.context
}

func (owner *agentSessionProcessOwner) Event() agentlifecycle.Event {
	if owner == nil || owner.osOwner == nil {
		return ""
	}
	return owner.osOwner.Event()
}

func (owner *agentSessionProcessOwner) SetSource(source agent.ConnectionSource) error {
	if owner == nil || source == nil {
		return errors.New("agent lifecycle owner and connection source are required")
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.closing {
		return errors.New("agent lifecycle owner is closing")
	}
	if owner.source != nil {
		return errors.New("agent lifecycle owner already has a connection source")
	}
	owner.source = source
	return nil
}

func (owner *agentSessionProcessOwner) SetCrashGuard(guard ownedCrashGuard) error {
	if owner == nil || guard == nil || guard.Done() == nil {
		return errors.New("agent lifecycle owner and crash guard are required")
	}
	owner.mu.Lock()
	if owner.closing {
		owner.mu.Unlock()
		return errors.New("agent lifecycle owner is closing")
	}
	if owner.guard != nil {
		owner.mu.Unlock()
		return errors.New("agent lifecycle owner already has a crash guard")
	}
	owner.guard = guard
	owner.guardWatcherDone = make(chan struct{})
	done := owner.guardWatcherDone
	owner.mu.Unlock()
	go func() {
		defer close(done)
		select {
		case <-guard.Done():
			owner.mu.Lock()
			closing := owner.closing
			owner.mu.Unlock()
			if !closing {
				owner.cancel(errCrashGuardStopped)
			}
		case <-owner.context.Done():
		}
	}()
	return nil
}

func (owner *agentSessionProcessOwner) Close() agentSessionProcessCloseResult {
	if owner == nil {
		return agentSessionProcessCloseResult{}
	}
	owner.close.Do(func() {
		owner.mu.Lock()
		owner.closing = true
		source := owner.source
		guard := owner.guard
		guardWatcherDone := owner.guardWatcherDone
		owner.mu.Unlock()

		owner.cancel(context.Canceled)
		owner.osOwner.Stop()
		if source != nil {
			owner.closeResult.SourceErr = source.Close()
		}
		if guard != nil {
			owner.closeResult.GuardErr = guard.Close()
		}
		if guardWatcherDone != nil {
			<-guardWatcherDone
		}
		owner.closeResult.LifecycleErr = owner.osOwner.Close()
	})
	return owner.closeResult
}
