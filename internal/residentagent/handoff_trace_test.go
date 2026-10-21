package residentagent

import (
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestHandoffTraceIncludesObservedLocalCaptureDelay(t *testing.T) {
	// Paired g416 collection 8885f0e5: request/grant took 35.488ms, while
	// source capture activation took 223.420ms. The former trigger missed it.
	s := handoff.Summary{Role: "source", Exchange: &handoff.ExchangeProfile{Measured: 1}}
	s.Exchange.Micros[handoff.RequestWrittenToGrantRead] = 35488
	s.Preparation.Measured = uint64(1) << handoff.CaptureActivate
	s.Preparation.Micros[handoff.CaptureActivate] = 223420
	if !handoffNeedsTrace(s) {
		t.Fatal("slow measured source activation did not request a trace")
	}
	s.Preparation.Measured = 0
	if handoffNeedsTrace(s) {
		t.Fatal("unmeasured source activation requested a trace")
	}
	s.Preparation.Measured = uint64(1) << handoff.SourceDeactivate
	s.Preparation.Micros[handoff.SourceDeactivate] = 137952
	if !handoffNeedsTrace(s) {
		t.Fatal("slow measured source deactivation did not request a trace")
	}
	s.Role = "receiver"
	if handoffNeedsTrace(s) {
		t.Fatal("receiver summary was mistaken for local source evidence")
	}
}

func TestHandoffTracePreservesNetworkTriggerWithoutAddingNestedSpans(t *testing.T) {
	s := handoff.Summary{Role: "source", Exchange: &handoff.ExchangeProfile{Measured: 1}}
	s.Exchange.Micros[handoff.RequestWrittenToGrantRead] = 80000
	if !handoffNeedsTrace(s) {
		t.Fatal("existing slow exchange trigger lost")
	}
	s.Exchange.Measured = 0
	s.Preparation.Measured = uint64(1)<<handoff.CaptureActivate | uint64(1)<<handoff.SourceDeactivate
	s.Preparation.Micros[handoff.CaptureActivate] = 49084
	s.Preparation.Micros[handoff.SourceDeactivate] = 76275
	if handoffNeedsTrace(s) {
		t.Fatal("unmeasured exchange or sum of separate transitions requested a trace")
	}
}
