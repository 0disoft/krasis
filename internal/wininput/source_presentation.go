package wininput

import (
	"runtime"
	"sync"
	"time"
)

type SourcePresentationReporter func(operation, result string, elapsed time.Duration)

const sourcePresentationWait = 20 * time.Millisecond

type sourcePresentationState struct {
	mu       sync.Mutex
	window   uintptr
	sequence uint64
	retired  bool
	report   SourcePresentationReporter
}

type sourcePresentationRequest struct {
	state    *sourcePresentationState
	sequence uint64
	enabled  bool
	result   chan bool
}

func (r *sourcePresentationRequest) current() bool {
	r.state.mu.Lock()
	defer r.state.mu.Unlock()
	return !r.state.retired && r.state.sequence == r.sequence
}

func (s *sourcePresentationState) retire() {
	s.mu.Lock()
	s.retired = true
	s.sequence++
	s.mu.Unlock()
}

// Exactly one native worker per process, with one replaceable queued request.
// A stuck Shell call cannot accumulate goroutines or block the input owner.
// COM itself is not forcibly canceled. Late results are rejected by generation,
// and the newest desired state is applied next on the same serialized worker.
type sourcePresentationBroker struct {
	mu     sync.Mutex
	latest *sourcePresentationRequest
	wake   chan struct{}
	apply  func(uintptr, bool) bool
}

func newSourcePresentationBroker(apply func(uintptr, bool) bool) *sourcePresentationBroker {
	b := &sourcePresentationBroker{wake: make(chan struct{}, 1), apply: apply}
	go b.run()
	return b
}

func (b *sourcePresentationBroker) request(s *sourcePresentationState, enabled bool, wait time.Duration) bool {
	started := time.Now()
	op := "clear"
	if enabled {
		op = "mark"
	}
	result := "superseded"
	defer func() {
		if s.report != nil {
			s.report(op, result, time.Since(started))
		}
	}()
	s.mu.Lock()
	if s.retired {
		s.mu.Unlock()
		return false
	}
	s.sequence++
	r := &sourcePresentationRequest{state: s, sequence: s.sequence, enabled: enabled, result: make(chan bool, 1)}
	s.mu.Unlock()
	b.mu.Lock()
	if previous := b.latest; previous != nil {
		previous.result <- false
	}
	b.latest = r
	b.mu.Unlock()
	select {
	case b.wake <- struct{}{}:
	default:
	}
	timer := time.NewTimer(wait)
	defer timer.Stop()
	select {
	case accepted := <-r.result:
		if !r.current() {
			return false
		}
		result = "failed"
		if accepted {
			result = "accepted"
		}
		return accepted
	case <-timer.C:
		result = "timeout"
		return false
	}
}

func (b *sourcePresentationBroker) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	for range b.wake {
		b.mu.Lock()
		r := b.latest
		b.latest = nil
		b.mu.Unlock()
		if r == nil {
			continue
		}
		accepted := false
		if r.current() {
			started := time.Now()
			accepted = b.apply(r.state.window, r.enabled)
			result := "superseded"
			if r.current() {
				result = "failed"
				if accepted {
					result = "accepted"
				}
			} else {
				accepted = false
			}
			if r.state.report != nil {
				op := "clear_native"
				if r.enabled {
					op = "mark_native"
				}
				r.state.report(op, result, time.Since(started))
			}
		}
		r.result <- accepted
	}
}
