package wininput

import (
	"sync"
	"sync/atomic"
	"time"
)

// A stuck optional consumer owns no input/native resource. At most one can
// survive in this process, even across repeated capture-owner replacement.
var hookTimingConsumer atomic.Bool
var hookTimingDropped atomic.Uint64

func HookTimingDroppedTotal() uint64 { return hookTimingDropped.Load() }

type hookTimingSample struct {
	at        time.Time
	completed bool
}
type hookTimingRelay struct {
	queue           chan hookTimingSample
	raw             chan time.Time
	slots           chan struct{}
	stop, done      chan struct{}
	once            sync.Once
	pending         [16]bool // only the low-level hook thread accesses these fields
	depth, overflow int
}

func newHookTimingRelay(next HookTimingObserver) *hookTimingRelay {
	if next == nil {
		return nil
	}
	r := &hookTimingRelay{queue: make(chan hookTimingSample, 256), raw: make(chan time.Time, 64), slots: make(chan struct{}, 128), stop: make(chan struct{}), done: make(chan struct{})}
	if !hookTimingConsumer.CompareAndSwap(false, true) {
		r.Close()
		close(r.done)
		return r
	}
	go func() {
		defer close(r.done)
		defer hookTimingConsumer.Store(false)
		defer func() { _ = recover() }()
		dispatch := func(s hookTimingSample) {
			if s.completed {
				next.ObserveHookCompleted(s.at)
				<-r.slots
			} else {
				next.ObserveHookStarted(s.at)
			}
		}
		defer func() {
			// Already accepted completions retain their actual timestamp on clean
			// shutdown. Draining is diagnostic-only and never delays native teardown.
			for {
				select {
				case s := <-r.queue:
					dispatch(s)
				case at := <-r.raw:
					next.ObserveRawInput(at)
				default:
					return
				}
			}
		}()
		for {
			select {
			case <-r.stop:
				return
			case at := <-r.raw:
				next.ObserveRawInput(at)
			case s := <-r.queue:
				dispatch(s)
			}
		}
	}()
	return r
}
func (r *hookTimingRelay) stopped() bool {
	select {
	case <-r.stop:
		return true
	case <-r.done:
		return true
	default:
		return false
	}
}
func (r *hookTimingRelay) ObserveHookStarted(at time.Time) {
	if r == nil {
		return
	}
	if r.depth == len(r.pending) {
		r.overflow++
		hookTimingDropped.Add(1)
		return
	}
	slot := r.depth
	r.depth++
	r.pending[slot] = false
	if r.stopped() {
		hookTimingDropped.Add(1)
		return
	}
	select {
	case r.slots <- struct{}{}:
		// Two queue slots are reserved per outstanding callback. Raw samples use
		// their own queue and cannot consume the completion's reserved slot.
		r.pending[slot] = true
		r.queue <- hookTimingSample{at: at}
	default:
		hookTimingDropped.Add(1)
	}
}
func (r *hookTimingRelay) ObserveHookCompleted(at time.Time) {
	if r == nil {
		return
	}
	if r.overflow > 0 {
		r.overflow--
		return
	}
	if r.depth == 0 {
		return
	}
	r.depth--
	if r.pending[r.depth] {
		r.pending[r.depth] = false
		r.queue <- hookTimingSample{at: at, completed: true}
	}
}
func (r *hookTimingRelay) ObserveRawInput(at time.Time) {
	if r == nil {
		return
	}
	if r.stopped() {
		hookTimingDropped.Add(1)
		return
	}
	select {
	case r.raw <- at:
	default:
		hookTimingDropped.Add(1)
	}
}
func (r *hookTimingRelay) Close() {
	if r != nil {
		r.once.Do(func() { close(r.stop) })
	}
}
