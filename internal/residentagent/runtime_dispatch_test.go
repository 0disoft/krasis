package residentagent

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/transport"
)

func TestSlowDispatchKeepsSameOperationRatherThanUnrelatedInjectionMaximum(t *testing.T) {
	d := newRuntimeDiagnostics()
	at := d.started.Add(time.Second)
	d.ObserveReadCompleted(at)
	d.ObserveDispatchStarted(transport.MessageInput, at.Add(time.Millisecond))
	d.ObserveDispatchCoordinatorWaiting(at.Add(2 * time.Millisecond))
	d.ObserveDispatchCoordinatorAcquired(at.Add(152 * time.Millisecond))
	d.ObserveDestinationInjectionStarted(at.Add(170 * time.Millisecond))
	d.ObserveDestinationInjectionCompleted(at.Add(172 * time.Millisecond))
	d.ObserveDispatchCompleted(at.Add(201 * time.Millisecond))
	// This later dispatch has a larger injection, but a smaller whole duration.
	at = at.Add(time.Second)
	d.ObserveReadCompleted(at)
	d.ObserveDispatchStarted(transport.MessageInput, at)
	d.ObserveDestinationInjectionStarted(at)
	d.ObserveDestinationInjectionCompleted(at.Add(80 * time.Millisecond))
	d.ObserveDispatchCompleted(at.Add(90 * time.Millisecond))
	s := d.snapshot(at.Add(time.Second))
	p := s.SlowDispatch[1]
	if p.Kind != "input" || p.DurationMicros != 200000 || !p.ReadMeasured || p.ReadToStartMicros != 1000 ||
		!p.CoordinatorMeasured || p.CoordinatorWaitMicros != 150000 || p.AfterCoordinatorMicros != 49000 ||
		p.InjectionMicros != 2000 || p.InjectionMaxMicros != 2000 || p.Injections != 1 || s.InjectionMaxMicros != 80000 {
		t.Fatalf("mixed independent operations: %+v", s)
	}
	if next := d.snapshot(at.Add(2 * time.Second)); next.SlowDispatch[1].Kind != "" {
		t.Fatal("completed evidence repeated in next interval", next)
	}
}

func TestSlowDispatchSeparatesControlAndUnknownCoordinatorFromInput(t *testing.T) {
	d := newRuntimeDiagnostics()
	at := d.started.Add(time.Second)
	d.ObserveDispatchStarted(transport.MessageControl, at)
	d.ObserveDispatchCompleted(at.Add(197 * time.Millisecond))
	d.ObserveReadConnectionStarted()
	at = at.Add(time.Second)
	d.ObserveDispatchStarted(transport.MessageInput, at)
	d.ObserveDispatchCoordinatorWaiting(at)
	// Cancellation while waiting must not invent an acquired gate.
	d.ObserveDispatchCompleted(at.Add(80 * time.Millisecond))
	s := d.snapshot(at.Add(time.Second))
	if s.SlowDispatch[0].DurationMicros != 197000 || s.SlowDispatch[0].CoordinatorMeasured ||
		s.SlowDispatch[1].CoordinatorMeasured || s.SlowDispatch[1].ReadMeasured || s.SlowDispatch[1].Injections != 0 {
		t.Fatalf("kind or availability lost: %+v", s)
	}
}

func TestSlowDispatchCallbackIsBoundedAndAllocationFree(t *testing.T) {
	d := newRuntimeDiagnostics()
	at := d.started.Add(time.Second)
	if n := testing.AllocsPerRun(100, func() {
		d.ObserveReadCompleted(at)
		d.ObserveDispatchStarted(transport.MessageHeartbeat, at)
		d.ObserveDispatchCoordinatorWaiting(at)
		d.ObserveDispatchCoordinatorAcquired(at.Add(time.Millisecond))
		d.ObserveDispatchCompleted(at.Add(100 * time.Millisecond))
	}); n != 0 {
		t.Fatal("callback allocations", n)
	}
	if len(d.slowDispatch) != cap(d.slowDispatch) {
		t.Fatal("evidence queue is not bounded")
	}
	s := d.snapshot(at.Add(time.Second))
	if s.SlowDispatchDropped == 0 || s.SlowDispatch[2].Kind != "heartbeat" || len(d.slowDispatch) != 0 {
		t.Fatal("queue loss was silent or queue was not drained", s)
	}
}
