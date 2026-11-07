package agentlifecycle

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
)

var ErrMonitorStopped = errors.New("agent lifecycle monitor stopped unexpectedly")

type Monitor interface {
	Events() <-chan Event
	Done() <-chan struct{}
	Close() error
}

type StartMonitor func() (Monitor, error)

type Owner struct {
	context context.Context
	cancel  context.CancelCauseFunc
	monitor Monitor
	done    chan struct{}

	closing    atomic.Bool
	mu         sync.Mutex
	event      Event
	close      sync.Once
	closeErr   error
	transition func(Event) bool
}

func Start(parent context.Context, start StartMonitor) (*Owner, error) {
	if parent == nil {
		return nil, errors.New("agent lifecycle parent context is required")
	}
	if start == nil {
		return nil, errors.New("agent lifecycle monitor starter is required")
	}
	if err := parent.Err(); err != nil {
		return nil, err
	}
	monitor, err := start()
	if err != nil {
		return nil, err
	}
	if monitor == nil || monitor.Events() == nil || monitor.Done() == nil {
		if monitor != nil {
			_ = monitor.Close()
		}
		return nil, errors.New("agent lifecycle monitor is invalid")
	}
	ctx, cancel := context.WithCancelCause(parent)
	owner := &Owner{context: ctx, cancel: cancel, monitor: monitor, done: make(chan struct{})}
	go owner.watch(parent)
	return owner, nil
}

func (owner *Owner) Context() context.Context {
	if owner == nil {
		return nil
	}
	return owner.context
}

func (owner *Owner) Event() Event {
	if owner == nil {
		return ""
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	return owner.event
}

// A product session can retire and rebuild its verified protected input path
// without retiring the process. All other events still stop the process, and
// an unhandled transition retains the default fail-closed behavior.
func (owner *Owner) SetSessionTransitionHandler(handler func(Event) bool) {
	owner.mu.Lock()
	defer owner.mu.Unlock()
	owner.transition = handler
}

func (owner *Owner) Close() error {
	if owner == nil {
		return nil
	}
	owner.close.Do(func() {
		owner.Stop()
		owner.closeErr = owner.monitor.Close()
		<-owner.done
	})
	return owner.closeErr
}

func (owner *Owner) Stop() {
	if owner == nil {
		return
	}
	owner.closing.Store(true)
	owner.cancel(context.Canceled)
}

func (owner *Owner) watch(parent context.Context) {
	defer close(owner.done)
	for {
		select {
		case event, ok := <-owner.monitor.Events():
			if !ok {
				owner.stopForMonitorExit(parent)
				return
			}
			monitorStopped := false
			select {
			case <-owner.monitor.Done():
				monitorStopped = true
			default:
			}
			if owner.handleEvent(event, monitorStopped) {
				return
			}
		case <-owner.monitor.Done():
			// The monitor can publish its last event immediately before closing
			// Done. Drain already-published events before classifying the exit.
			for {
				select {
				case event, ok := <-owner.monitor.Events():
					if ok {
						if owner.handleEvent(event, true) {
							return
						}
						continue
					}
				default:
				}
				break
			}
			owner.stopForMonitorExit(parent)
			return
		case <-parent.Done():
			return
		}
	}
}

func (owner *Owner) handleEvent(event Event, monitorStopped bool) bool {
	if event.Validate() != nil {
		owner.cancel(ErrMonitorStopped)
		return true
	}
	owner.mu.Lock()
	handler := owner.transition
	owner.mu.Unlock()
	if !monitorStopped && (event == EventSessionLock || event == EventDesktopSwitch) && handler != nil && handler(event) {
		return false
	}
	owner.mu.Lock()
	if owner.event == "" {
		owner.event = event
	}
	owner.mu.Unlock()
	owner.cancel(&TransitionError{Event: event})
	return true
}

func (owner *Owner) stopForMonitorExit(parent context.Context) {
	if !owner.closing.Load() && parent.Err() == nil {
		owner.cancel(ErrMonitorStopped)
	}
}
