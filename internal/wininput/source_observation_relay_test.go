package wininput

import (
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type blockedSourceObserver struct {
	SourcePipelineObserver
	entered, release chan struct{}
	once             sync.Once
}

func (o *blockedSourceObserver) ObserveWheelHook() {
	o.once.Do(func() { close(o.entered); <-o.release })
}

func waitSourceObservations(t *testing.T, r *SourceObservationRelay) {
	t.Helper()
	select {
	case <-r.done:
	case <-time.After(time.Second):
		t.Fatal("released observation worker did not retire")
	}
}

func TestBlockedSourceObservationsCannotBlockPhysicalRoutingOrAccumulateConsumers(t *testing.T) {
	var relays []*SourceObservationRelay
	var observers []*blockedSourceObserver
	defer func() {
		for i, r := range relays {
			r.Close()
			close(observers[i].release)
			waitSourceObservations(t, r)
		}
	}()
	for range cap(sourceObservationConsumers) {
		o := &blockedSourceObserver{SourcePipelineObserver: &recordingSourcePipelineObserver{}, entered: make(chan struct{}), release: make(chan struct{})}
		r := NewSourceObservationRelay(o, nil)
		relays = append(relays, r)
		observers = append(observers, o)
		r.Pipeline().ObserveWheelHook()
		select {
		case <-o.entered:
		case <-time.After(time.Second):
			t.Fatal("consumer did not start")
		}
	}
	r := relays[0]
	before := SourceObservationDroppedTotal()
	finished := make(chan struct{})
	go func() {
		for range 10000 {
			r.Pipeline().ObserveActiveHookMouseMove()
		}
		close(finished)
	}()
	select {
	case <-finished:
	case <-time.After(time.Second):
		t.Fatal("full diagnostic queue blocked producer")
	}
	if len(r.events) > 256 || SourceObservationDroppedTotal() <= before {
		t.Fatal("diagnostic overload was not bounded/visible")
	}
	writer := r.Pipeline()
	if n := testing.AllocsPerRun(100, func() { writer.ObserveWheelMapped() }); n != 0 {
		t.Fatal("callback handoff allocates", n)
	}
	// A blocked observer on the actual session route cannot alter suppression
	// or discrete input delivery to its current sender.
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 2)}
	s := startObservedSourceSession(t, 16, sink, writer)
	for _, kind := range []control.InputActionKind{control.InputKeyDown, control.InputKeyUp} {
		action := control.InputAction{Kind: kind, Code: 30}
		if !routePhysicalAction(s, nil, action, 0) {
			t.Fatal("diagnostic stall changed suppression")
		}
		if got := receiveAction(t, sink.sent); got != action {
			t.Fatal("diagnostic stall changed input order")
		}
	}
	for range 10 {
		replacement := NewSourceObservationRelay(observers[0], nil)
		replacement.Pipeline().ObserveWheelHook()
		replacement.Close()
		if len(replacement.events) != 0 {
			t.Fatal("blocked observers spawned replacement work")
		}
	}
}

func TestSourceObservationsPreserveResultsAndSeparateQueueConsumer(t *testing.T) {
	pipeline, queue := &recordingSourcePipelineObserver{}, &recordingSourcePipelineObserver{}
	r := NewSourceObservationRelay(pipeline, queue)
	r.Pipeline().ObserveWheelQueue(SourceQueueRejectedOverflow)
	r.Pipeline().ObserveWheelForegroundFallback(SourceForegroundFallbackVerificationLost)
	r.Pipeline().ObserveSourceQueueEnqueued(2)
	r.Queue().ObserveSourceQueueEnqueued(7)
	r.Queue().ObserveSourceQueueDequeued(3 * time.Millisecond)
	r.Queue().ObserveSourceQueueDiscardedStale()
	r.Queue().(SourceQueueCoalescedObserver).ObserveSourceQueueCoalesced()
	r.Close()
	waitSourceObservations(t, r)
	if pipeline.rejectedOverflow != 1 || len(pipeline.foregroundFallbacks) != 1 || pipeline.foregroundFallbacks[0] != SourceForegroundFallbackVerificationLost || pipeline.queueEnqueued != 1 || pipeline.maxQueueDepth != 2 {
		t.Fatal("pipeline outcomes changed", pipeline)
	}
	if queue.queueEnqueued != 1 || queue.maxQueueDepth != 7 || queue.maxResidence != 3*time.Millisecond || queue.queueCoalesced != 1 || queue.queueDiscardedStale != 1 || pipeline.queueDequeued != 0 {
		t.Fatal("queue diagnostics duplicated or changed", pipeline, queue)
	}
}

func TestSourcePresentationObserverPanicRetiresOnlyDiagnosticWorker(t *testing.T) {
	r := newSourceObservationRelay(nil, nil, func(string, string, time.Duration) { panic("private data") })
	before := SourceObservationDroppedTotal()
	r.presentationReporter()("return_hide", "accepted", time.Millisecond)
	waitSourceObservations(t, r)
	if SourceObservationDroppedTotal() <= before {
		t.Fatal("failed consumer was not observable")
	}
	r.Close()
	r.Close()
	var empty *SourceObservationRelay
	empty.Close()
	if empty.Pipeline() != nil || empty.Queue() != nil || empty.presentationReporter() != nil {
		t.Fatal("nil observers introduced work")
	}
}
