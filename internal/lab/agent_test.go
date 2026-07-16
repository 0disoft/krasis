package lab

import "testing"

func TestWheelPipelineRawWheelUnforwarded(t *testing.T) {
	tests := []struct {
		name  string
		wheel WheelPipelineEvidence
		want  bool
	}{
		{name: "no raw wheel", wheel: WheelPipelineEvidence{}, want: false},
		{name: "raw wheel not sent", wheel: WheelPipelineEvidence{RawObserved: 1}, want: true},
		{name: "raw wheel sent", wheel: WheelPipelineEvidence{RawObserved: 4, Sent: 1}, want: false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := test.wheel.RawWheelUnforwarded(); got != test.want {
				t.Fatalf("RawWheelUnforwarded() = %t, want %t", got, test.want)
			}
		})
	}
}

func TestWheelPipelineWindowWheelAndCaptureLifecycle(t *testing.T) {
	wheel := WheelPipelineEvidence{WindowObserved: 2}
	if !wheel.WindowWheelUnforwarded() {
		t.Fatal("WindowWheelUnforwarded() = false")
	}
	wheel.Sent = 1
	if wheel.WindowWheelUnforwarded() {
		t.Fatal("WindowWheelUnforwarded() = true after send")
	}

	for _, test := range []struct {
		name  string
		wheel WheelPipelineEvidence
		want  bool
	}{
		{name: "unused", wheel: WheelPipelineEvidence{}, want: true},
		{name: "restored", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestored: 1}, want: true},
		{name: "restore skipped", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreSkipped: 1}, want: true},
		{name: "leaked activation", wheel: WheelPipelineEvidence{CaptureActivations: 1}, want: false},
		{name: "missing restore classification", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1}, want: false},
		{name: "restore failed", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFailed: 1}, want: false},
	} {
		t.Run(test.name, func(t *testing.T) {
			if got := test.wheel.CaptureLifecycleSafe(); got != test.want {
				t.Fatalf("CaptureLifecycleSafe()=%t want=%t", got, test.want)
			}
		})
	}
}

func TestSourcePipelineRequiresCaptureForSourceActionsAndRejectsOverflow(t *testing.T) {
	source := SourceInputEvidence{PointerMoves: 1}
	if (SourcePipelineEvidence{}).ValidAgainst(source) {
		t.Fatal("source action without capture activation was valid")
	}
	valid := SourcePipelineEvidence{Wheel: WheelPipelineEvidence{
		CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreSkipped: 1,
	}}
	if !valid.ValidAgainst(source) {
		t.Fatal("balanced capture lifecycle was invalid")
	}
	overflow := valid
	overflow.Wheel.FocusRestoreSkipped = ^uint64(0)
	overflow.Wheel.FocusRestored = 1
	if overflow.ValidAgainst(source) {
		t.Fatal("overflowing focus classification was valid")
	}
}

func TestAgentEvidenceCheckedCountersRejectOverflow(t *testing.T) {
	if total, valid := (SourceInputEvidence{KeyDowns: ^uint64(0), KeyUps: 1}).CheckedTotalActions(); valid || total != 0 {
		t.Fatalf("source total = %d valid=%t", total, valid)
	}
	if total, valid := (InputEvidence{PointerMoves: ^uint64(0), Wheels: 1}).CheckedTotalActions(); valid || total != 0 {
		t.Fatalf("destination total = %d valid=%t", total, valid)
	}
	if total, valid := (InjectionFailureEvidence{NotApplied: ^uint64(0), PartiallyApplied: 1}).CheckedTotal(); valid || total != 0 {
		t.Fatalf("injection total = %d valid=%t", total, valid)
	}
	if total, valid := (CursorRollbackEvidence{Restored: ^uint64(0), Failed: 1}).CheckedClassified(); valid || total != 0 {
		t.Fatalf("rollback total = %d valid=%t", total, valid)
	}
	if (SourcePipelineEvidence{}).ValidAgainst(SourceInputEvidence{KeyDowns: ^uint64(0), KeyUps: 1}) {
		t.Fatal("source pipeline accepted overflowing source counters")
	}
}
