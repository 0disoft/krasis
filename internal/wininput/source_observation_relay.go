package wininput

import (
	"sync/atomic"
	"time"
)

// Optional diagnostics never own input or native resources. A blocked consumer
// must not retain the hook, input sender or an unlimited chain of replacements.
var sourceObservationConsumers = make(chan struct{}, 4)
var sourceObservationDropped atomic.Uint64

func SourceObservationDroppedTotal() uint64 { return sourceObservationDropped.Load() }

type sourceObservationKind uint8

const (
	sourceObservedWheelHook sourceObservationKind = iota
	sourceObservedWheelSynthesized
	sourceObservedWheelRaw
	sourceObservedWheelWindow
	sourceObservedCaptureActivated
	sourceObservedCaptureDeactivated
	sourceObservedForegroundFallback
	sourceObservedFocusFallback
	sourceObservedFocusRestore
	sourceObservedPostHideFocus
	sourceObservedHookMove
	sourceObservedHookButton
	sourceObservedWheelMapped
	sourceObservedWheelQueue
	sourceObservedQueueEnqueued
	sourceObservedQueueDequeued
	sourceObservedQueueStale
	sourceObservedQueueCoalesced
	sourceObservedPresentation
)

type sourceObservation struct {
	kind              sourceObservationKind
	value             int64
	queueOnly         bool
	operation, result string
}

type SourceObservationRelay struct {
	pipeline     SourcePipelineObserver
	queue        SourceQueueObserver
	presentation SourcePresentationReporter
	events       chan sourceObservation
	stop, done   chan struct{}
	closed       atomic.Bool
}

// NewSourceObservationRelay isolates optional observers from production input.
// Samples are bounded and may be dropped; they never contain raw input data.
func NewSourceObservationRelay(pipeline SourcePipelineObserver, queue SourceQueueObserver) *SourceObservationRelay {
	return newSourceObservationRelay(pipeline, queue, nil)
}

func newSourceObservationRelay(pipeline SourcePipelineObserver, queue SourceQueueObserver, presentation SourcePresentationReporter) *SourceObservationRelay {
	if pipeline == nil && queue == nil && presentation == nil {
		return nil
	}
	r := &SourceObservationRelay{pipeline: pipeline, queue: queue, presentation: presentation, events: make(chan sourceObservation, 256), stop: make(chan struct{}), done: make(chan struct{})}
	select {
	case sourceObservationConsumers <- struct{}{}:
		go r.run()
	default:
		r.closed.Store(true)
		close(r.stop)
		close(r.done)
	}
	return r
}

func (r *SourceObservationRelay) Pipeline() SourcePipelineObserver {
	if r == nil || r.pipeline == nil {
		return nil
	}
	return sourceObservationWriter{relay: r}
}

func (r *SourceObservationRelay) Queue() SourceQueueObserver {
	if r == nil || r.queue == nil {
		return nil
	}
	return sourceObservationWriter{relay: r, queueOnly: true}
}

func (r *SourceObservationRelay) presentationReporter() SourcePresentationReporter {
	if r == nil || r.presentation == nil {
		return nil
	}
	return func(operation, result string, elapsed time.Duration) {
		r.offer(sourceObservation{kind: sourceObservedPresentation, operation: operation, result: result, value: int64(elapsed)})
	}
}

func (r *SourceObservationRelay) Close() {
	if r != nil && !r.closed.Swap(true) {
		close(r.stop)
	}
}

func (r *SourceObservationRelay) run() {
	defer close(r.done)
	defer func() {
		<-sourceObservationConsumers
		if recover() != nil {
			sourceObservationDropped.Add(1)
		}
	}()
	for {
		select {
		case event := <-r.events:
			r.dispatch(event)
		case <-r.stop:
			// Drain queued diagnostics without making input/native teardown
			// join this optional consumer. A stuck callback retains one slot.
			for {
				select {
				case event := <-r.events:
					r.dispatch(event)
				default:
					return
				}
			}
		}
	}
}

func (r *SourceObservationRelay) dispatch(e sourceObservation) {
	if e.kind == sourceObservedPresentation {
		r.presentation(e.operation, e.result, time.Duration(e.value))
		return
	}
	if e.queueOnly {
		switch e.kind {
		case sourceObservedQueueEnqueued:
			r.queue.ObserveSourceQueueEnqueued(uint64(e.value))
		case sourceObservedQueueDequeued:
			r.queue.ObserveSourceQueueDequeued(time.Duration(e.value))
		case sourceObservedQueueStale:
			r.queue.ObserveSourceQueueDiscardedStale()
		case sourceObservedQueueCoalesced:
			if next, ok := r.queue.(SourceQueueCoalescedObserver); ok {
				next.ObserveSourceQueueCoalesced()
			}
		}
		return
	}
	switch e.kind {
	case sourceObservedWheelHook:
		r.pipeline.ObserveWheelHook()
	case sourceObservedWheelSynthesized:
		r.pipeline.ObserveWheelSynthesized()
	case sourceObservedWheelRaw:
		r.pipeline.ObserveWheelRaw()
	case sourceObservedWheelWindow:
		r.pipeline.ObserveWheelWindow()
	case sourceObservedCaptureActivated:
		r.pipeline.ObserveWheelCaptureActivated()
	case sourceObservedCaptureDeactivated:
		r.pipeline.ObserveWheelCaptureDeactivated()
	case sourceObservedForegroundFallback:
		r.pipeline.ObserveWheelForegroundFallback(SourceForegroundFallbackResult(e.value))
	case sourceObservedFocusFallback:
		r.pipeline.ObserveWheelFocusRestoreFallback(SourceFocusRestoreFallbackResult(e.value))
	case sourceObservedFocusRestore:
		r.pipeline.ObserveWheelFocusRestore(SourceFocusRestoreResult(e.value))
	case sourceObservedPostHideFocus:
		r.pipeline.ObserveWheelPostHideFocus(SourcePostHideFocusResult(e.value))
	case sourceObservedHookMove:
		r.pipeline.ObserveActiveHookMouseMove()
	case sourceObservedHookButton:
		r.pipeline.ObserveActiveHookMouseButton()
	case sourceObservedWheelMapped:
		r.pipeline.ObserveWheelMapped()
	case sourceObservedWheelQueue:
		r.pipeline.ObserveWheelQueue(SourceQueueOfferResult(e.value))
	case sourceObservedQueueEnqueued:
		r.pipeline.ObserveSourceQueueEnqueued(uint64(e.value))
	case sourceObservedQueueDequeued:
		r.pipeline.ObserveSourceQueueDequeued(time.Duration(e.value))
	case sourceObservedQueueStale:
		r.pipeline.ObserveSourceQueueDiscardedStale()
	}
}

type sourceObservationWriter struct {
	relay     *SourceObservationRelay
	queueOnly bool
}

func (w sourceObservationWriter) offer(kind sourceObservationKind, value int64) {
	w.relay.offer(sourceObservation{kind: kind, value: value, queueOnly: w.queueOnly})
}

func (r *SourceObservationRelay) offer(event sourceObservation) {
	if r.closed.Load() {
		sourceObservationDropped.Add(1)
		return
	}
	select {
	case <-r.done:
		sourceObservationDropped.Add(1)
	default:
		select {
		case r.events <- event:
			// A producer racing shutdown can arrive after the final drain.
			// Mark that sample uncertain even if it happened to be consumed.
			if r.closed.Load() {
				sourceObservationDropped.Add(1)
			}
		default:
			sourceObservationDropped.Add(1)
		}
	}
}

func (w sourceObservationWriter) ObserveWheelHook() { w.offer(sourceObservedWheelHook, 0) }
func (w sourceObservationWriter) ObserveWheelSynthesized() {
	w.offer(sourceObservedWheelSynthesized, 0)
}
func (w sourceObservationWriter) ObserveWheelRaw()    { w.offer(sourceObservedWheelRaw, 0) }
func (w sourceObservationWriter) ObserveWheelWindow() { w.offer(sourceObservedWheelWindow, 0) }
func (w sourceObservationWriter) ObserveWheelCaptureActivated() {
	w.offer(sourceObservedCaptureActivated, 0)
}
func (w sourceObservationWriter) ObserveWheelCaptureDeactivated() {
	w.offer(sourceObservedCaptureDeactivated, 0)
}
func (w sourceObservationWriter) ObserveWheelForegroundFallback(v SourceForegroundFallbackResult) {
	w.offer(sourceObservedForegroundFallback, int64(v))
}
func (w sourceObservationWriter) ObserveWheelFocusRestoreFallback(v SourceFocusRestoreFallbackResult) {
	w.offer(sourceObservedFocusFallback, int64(v))
}
func (w sourceObservationWriter) ObserveWheelFocusRestore(v SourceFocusRestoreResult) {
	w.offer(sourceObservedFocusRestore, int64(v))
}
func (w sourceObservationWriter) ObserveWheelPostHideFocus(v SourcePostHideFocusResult) {
	w.offer(sourceObservedPostHideFocus, int64(v))
}
func (w sourceObservationWriter) ObserveActiveHookMouseMove()   { w.offer(sourceObservedHookMove, 0) }
func (w sourceObservationWriter) ObserveActiveHookMouseButton() { w.offer(sourceObservedHookButton, 0) }
func (w sourceObservationWriter) ObserveWheelMapped()           { w.offer(sourceObservedWheelMapped, 0) }
func (w sourceObservationWriter) ObserveWheelQueue(v SourceQueueOfferResult) {
	w.offer(sourceObservedWheelQueue, int64(v))
}
func (w sourceObservationWriter) ObserveSourceQueueEnqueued(v uint64) {
	w.offer(sourceObservedQueueEnqueued, int64(v))
}
func (w sourceObservationWriter) ObserveSourceQueueDequeued(v time.Duration) {
	w.offer(sourceObservedQueueDequeued, int64(v))
}
func (w sourceObservationWriter) ObserveSourceQueueDiscardedStale() {
	w.offer(sourceObservedQueueStale, 0)
}
func (w sourceObservationWriter) ObserveSourceQueueCoalesced() {
	w.offer(sourceObservedQueueCoalesced, 0)
}
