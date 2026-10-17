package residentagent

import (
	"time"

	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/transport"
)

func (d *runtimeDiagnostics) ObserveDispatchCoordinatorWaiting(at time.Time) {
	d.coordinatorQueued.Store(d.stamp(at))
}

func (d *runtimeDiagnostics) ObserveDispatchCoordinatorAcquired(at time.Time) {
	d.coordinatorAcquired.Store(d.stamp(at))
}

// The serial session reader owns dispatch and its synchronous injection
// callbacks. Publish only scalars to a fixed nonblocking queue. No logging,
// allocation, runtime query or wait is added to input/heartbeat callbacks.
func (d *runtimeDiagnostics) observeSlowDispatch(start, end int64) {
	if end-start < 50_000 {
		return
	}
	s := productlog.DispatchSpan{Kind: "unknown", StartedMicros: start, DurationMicros: end - start,
		InjectionMicros: d.dispatchInjected.Load(), InjectionMaxMicros: d.dispatchInjectionMax.Load(), Injections: d.dispatchInjections.Load()}
	switch transport.MessageKind(d.kind.Load()) {
	case transport.MessageControl:
		s.Kind = "control"
	case transport.MessageInput:
		s.Kind = "input"
	case transport.MessageHeartbeat:
		s.Kind = "heartbeat"
	case transport.MessageDisconnect:
		s.Kind = "disconnect"
	}
	if read := d.dispatchRead.Load(); read > 0 && read <= start {
		s.ReadMeasured, s.ReadToStartMicros = true, start-read
	}
	queued, acquired := d.coordinatorQueued.Load(), d.coordinatorAcquired.Load()
	if queued >= start && acquired >= queued && acquired <= end {
		s.CoordinatorMeasured = true
		s.CoordinatorWaitMicros = acquired - queued
		s.AfterCoordinatorMicros = end - acquired
	}
	select {
	case d.slowDispatch <- s:
	default:
		d.slowDispatchDropped.Add(1)
	}
}

func (d *runtimeDiagnostics) drainSlowDispatch(summary *productlog.RuntimeSummary) {
	// A concurrent producer cannot keep the sampler here indefinitely.
	for i := 0; i < cap(d.slowDispatch); i++ {
		select {
		case span := <-d.slowDispatch:
			index := productlog.DispatchKindIndex(span.Kind)
			if index >= 0 && span.DurationMicros > summary.SlowDispatch[index].DurationMicros {
				summary.SlowDispatch[index] = span
			}
		default:
			summary.SlowDispatchDropped = d.slowDispatchDropped.Swap(0)
			return
		}
	}
	summary.SlowDispatchDropped = d.slowDispatchDropped.Swap(0)
}
