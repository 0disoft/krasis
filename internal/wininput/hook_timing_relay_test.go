package wininput

import (
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type slowTimingObserver struct {
	entered, release chan struct{}
	once             sync.Once
	starts, ends     atomic.Uint64
}

func (o *slowTimingObserver) ObserveHookStarted(time.Time) {
	o.once.Do(func() { close(o.entered); <-o.release })
	o.starts.Add(1)
}
func (o *slowTimingObserver) ObserveHookCompleted(time.Time) { o.ends.Add(1) }
func (*slowTimingObserver) ObserveRawInput(time.Time)        {}

func TestBlockedTimingObserverCannotBlockHookOrSpawnReplacementConsumers(t *testing.T) {
	o := &slowTimingObserver{entered: make(chan struct{}), release: make(chan struct{})}
	r := newHookTimingRelay(o)
	t.Cleanup(func() { r.Close() })
	p := &sourceHookProgress{origin: time.Now(), next: r}
	callback := timedSourceCallback(func(int, uintptr, uintptr) uintptr { return 42 }, p)
	if callback(0, 0, 0) != 42 {
		t.Fatal("disposition changed")
	}
	<-o.entered
	before := HookTimingDroppedTotal()
	for i := 0; i < 10000; i++ {
		callback(0, 0, 0)
		r.ObserveRawInput(time.Now())
	}
	if n := testing.AllocsPerRun(100, func() { callback(0, 0, 0); r.ObserveRawInput(time.Now()) }); n != 0 {
		t.Fatal("callback diagnostic handoff allocates", n)
	}
	// Nested callbacks also reserve both timestamps without waiting or losing
	// the outer completion. Nesting beyond the fixed ledger is counted/dropped.
	for i := 0; i < 32; i++ {
		r.ObserveHookStarted(time.Now())
	}
	for i := 0; i < 32; i++ {
		r.ObserveHookCompleted(time.Now())
	}
	if p.active.Load() != 0 || p.failure(time.Now().Add(time.Hour)) != nil {
		t.Fatal("diagnostic stall became capture stall")
	}
	if HookTimingDroppedTotal() <= before || len(r.queue) > 256 || len(r.raw) > 64 {
		t.Fatal("unbounded diagnostic work")
	}
	r.Close()
	r.Close()
	replacement := newHookTimingRelay(o)
	for i := 0; i < 100; i++ {
		replacement.ObserveHookStarted(time.Now())
		replacement.ObserveHookCompleted(time.Now())
	}
	replacement.Close()
	if len(replacement.queue) != 0 {
		t.Fatal("blocked consumer created another worker")
	}
	close(o.release)
	select {
	case <-r.done:
	case <-time.After(time.Second):
		t.Fatal("released diagnostic consumer did not retire")
	}
	if o.starts.Load() != o.ends.Load() {
		t.Fatal("clean shutdown lost accepted completion")
	}
	if r.depth != 0 || r.overflow != 0 {
		t.Fatal("nested callback ledger retained")
	}
	if hookTimingConsumer.Load() {
		t.Fatal("diagnostic consumer gate retained")
	}
}

func TestSourceAbortFencesLateGrantWithoutWaitingForStateMutex(t *testing.T) {
	s := newInactiveSourceSession(t)
	s.started.Store(true)
	s.active = true
	s.generation = 7
	s.stateMu.Lock()
	defer s.stateMu.Unlock()
	cause := errors.New("capture failed")
	s.Abort(cause)
	if s.Active() || s.Offer(control.InputAction{Kind: control.InputKeyDown, Code: 1}) || s.current(7) || s.Activate() == nil {
		t.Fatal("retired callback still grants or suppresses")
	}
	if got := <-s.faults; got != cause {
		t.Fatal("failure reason lost", got)
	}
	var workers sync.WaitGroup
	for i := 0; i < 100; i++ {
		workers.Add(1)
		go func() { defer workers.Done(); s.Abort(cause) }()
	}
	workers.Wait()
	if len(s.faults) != 0 {
		t.Fatal("failure was republished")
	}
}
