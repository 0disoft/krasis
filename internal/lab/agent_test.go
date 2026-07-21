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
		{name: "fallback restored", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, ForegroundFallbackAttempted: 1, ForegroundFallbackSucceeded: 1, FocusRestored: 1}, want: true},
		{name: "focus fallback restored", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFallbackAttempted: 1, FocusRestoreFallbackSucceeded: 1, FocusRestored: 1}, want: true},
		{name: "restore skipped", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreSkipped: 1}, want: true},
		{name: "degraded restore recovered", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFallbackAttempted: 1, FocusRestoreFallbackAPIRejected: 1, FocusRestoreFailed: 1, FocusRestoreAPIRejected: 1, PostHideFocusAttempted: 1, PostHideFocusRecovered: 1}, want: true},
		{name: "degraded restore without foreground", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFallbackAttempted: 1, FocusRestoreFallbackAttachFailed: 1, FocusRestoreFailed: 1, FocusRestoreAPIRejected: 1, PostHideFocusAttempted: 1, PostHideFocusUnavailable: 1}, want: true},
		{name: "leaked activation", wheel: WheelPipelineEvidence{CaptureActivations: 1}, want: false},
		{name: "fallback outcome missing", wheel: WheelPipelineEvidence{ForegroundFallbackAttempted: 1}, want: false},
		{name: "fallback success without activation", wheel: WheelPipelineEvidence{ForegroundFallbackAttempted: 1, ForegroundFallbackSucceeded: 1}, want: false},
		{name: "fallback outcome without attempt", wheel: WheelPipelineEvidence{ForegroundFallbackAttachFailed: 1}, want: false},
		{name: "focus fallback outcome missing", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFallbackAttempted: 1, FocusRestored: 1}, want: false},
		{name: "focus fallback success without restore", wheel: WheelPipelineEvidence{FocusRestoreFallbackAttempted: 1, FocusRestoreFallbackSucceeded: 1}, want: false},
		{name: "focus fallback failure without restore failure", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFallbackAttempted: 1, FocusRestoreFallbackAttachFailed: 1, FocusRestoreSkipped: 1}, want: false},
		{name: "missing restore classification", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1}, want: false},
		{name: "restore API rejected", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFailed: 1, FocusRestoreAPIRejected: 1}, want: false},
		{name: "restore verification lost", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFailed: 1, FocusRestoreVerificationLost: 1}, want: false},
		{name: "post-hide outcome missing", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFailed: 1, FocusRestoreAPIRejected: 1, PostHideFocusAttempted: 1}, want: false},
		{name: "restore failure without fallback outcome", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFailed: 1, FocusRestoreAPIRejected: 1, PostHideFocusAttempted: 1, PostHideFocusRecovered: 1}, want: false},
		{name: "post-hide retained capture", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFailed: 1, FocusRestoreAPIRejected: 1, PostHideFocusAttempted: 1, PostHideFocusCaptureRetained: 1}, want: false},
		{name: "thread detach remains terminal", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFallbackAttempted: 1, FocusRestoreFallbackDetachFailed: 1, FocusRestoreFailed: 1, FocusRestoreAPIRejected: 1, PostHideFocusAttempted: 1, PostHideFocusRecovered: 1}, want: false},
		{name: "unclassified restore failure", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreFailed: 1}, want: false},
		{name: "failure subtype without total", wheel: WheelPipelineEvidence{CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreAPIRejected: 1}, want: false},
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
	valid := SourcePipelineEvidence{
		Wheel: WheelPipelineEvidence{
			CaptureActivations: 1, CaptureDeactivations: 1, FocusRestoreSkipped: 1,
		},
		Queue: SourceQueueEvidence{
			Enqueued: 1, Dequeued: 1, MaxDepth: 1, ResidenceSamples: 1, MaxResidenceMicros: 1,
		},
	}
	if !valid.ValidAgainst(source) {
		t.Fatal("balanced capture lifecycle was invalid")
	}
	overflow := valid
	overflow.Wheel.FocusRestoreSkipped = ^uint64(0)
	overflow.Wheel.FocusRestored = 1
	if overflow.ValidAgainst(source) {
		t.Fatal("overflowing focus classification was valid")
	}
	failureOverflow := valid
	failureOverflow.Wheel.FocusRestoreFailed = ^uint64(0)
	failureOverflow.Wheel.FocusRestoreAPIRejected = ^uint64(0)
	failureOverflow.Wheel.FocusRestoreVerificationLost = 1
	if failureOverflow.ValidAgainst(source) {
		t.Fatal("overflowing focus failure classification was valid")
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

func TestControlFlowEvidenceValidatesStageConservation(t *testing.T) {
	source := SourceInputEvidence{BoundaryIntents: 1, PointerMoves: 1}
	wheel := WheelPipelineEvidence{CaptureActivations: 1, ForegroundFallbackAttempted: 1, ForegroundFallbackSucceeded: 1}
	destination := InputEvidence{Entries: 1, ReturnArms: 1}
	evidence := ControlFlowEvidence{
		Boundary: BoundaryFlowEvidence{Queued: 1, Admitted: 1},
		Outbound: OutboundFlowEvidence{
			RequestWriteCompleted: 1, GrantReceived: 1, GrantApplySucceeded: 1,
			SourceSessionActivationSucceeded: 1, CaptureActivationAttempted: 1, CaptureActivationSucceeded: 1,
			CommitWriteCompleted: 1,
		},
		Inbound: InboundFlowEvidence{
			RequestReceived: 1, ReturnArmSucceeded: 1, EntryPrepareAttempted: 1, EntryPrepared: 1,
			GrantWriteCompleted: 1, CommitReceived: 1, EntryCommitted: 1,
		},
	}
	if !evidence.ValidAgainst(source, wheel, destination) {
		t.Fatal("valid control flow was rejected")
	}

	prepareFailure := ControlFlowEvidence{Inbound: InboundFlowEvidence{
		RequestReceived: 1, ReturnArmSucceeded: 1, EntryPrepareAttempted: 1, EntryRollbackAttempted: 1,
	}}
	if !prepareFailure.ValidAgainst(SourceInputEvidence{}, WheelPipelineEvidence{}, InputEvidence{ReturnArms: 1}) {
		t.Fatal("compensated entry prepare failure was rejected")
	}
	prepareFailure.Inbound.EntryPrepareAttempted = 2
	if prepareFailure.ValidAgainst(SourceInputEvidence{}, WheelPipelineEvidence{}, InputEvidence{ReturnArms: 1}) {
		t.Fatal("unclassified entry prepare attempt was accepted")
	}

	evidence.Outbound.CaptureActivationSucceeded = 0
	evidence.Outbound.CaptureActivationFailed = 1
	evidence.Outbound.CaptureActivationSetupFailed = 1
	evidence.Outbound.CommitWriteCompleted = 0
	evidence.Outbound.AbortWriteCompleted = 1
	if evidence.ValidAgainst(source, wheel, destination) {
		t.Fatal("capture success drift was accepted")
	}

	evidence.Outbound.CaptureActivationFailed = ^uint64(0)
	evidence.Outbound.CaptureActivationSetupFailed = ^uint64(0)
	evidence.Outbound.AbortWriteCompleted = ^uint64(0)
	evidence.Outbound.CaptureActivationSucceeded = 1
	if evidence.ValidAgainst(source, wheel, destination) {
		t.Fatal("overflowing capture classification was accepted")
	}

	evidence = ControlFlowEvidence{
		Boundary: BoundaryFlowEvidence{Queued: 1, Admitted: 1},
		Outbound: OutboundFlowEvidence{
			RequestWriteCompleted: 1, GrantReceived: 1, GrantApplySucceeded: 1,
			SourceSessionActivationFailed: 1, AbortWriteCompleted: 1,
		},
	}
	if !evidence.ValidAgainst(SourceInputEvidence{BoundaryIntents: 1}, WheelPipelineEvidence{}, InputEvidence{}) {
		t.Fatal("safely aborted source-session activation failure was rejected")
	}

	evidence = ControlFlowEvidence{
		Boundary: BoundaryFlowEvidence{Queued: 2, Admitted: 1},
		Outbound: OutboundFlowEvidence{RequestWriteCompleted: 1},
	}
	if evidence.ValidAgainst(SourceInputEvidence{BoundaryIntents: 2, PointerMoves: 1}, WheelPipelineEvidence{}, InputEvidence{}) {
		t.Fatal("unclassified boundary intent was accepted")
	}

	evidence = ControlFlowEvidence{
		Boundary: BoundaryFlowEvidence{Queued: 1, BlockedLocalMenu: 1},
	}
	if !evidence.ValidAgainst(SourceInputEvidence{BoundaryIntents: 1}, WheelPipelineEvidence{}, InputEvidence{}) {
		t.Fatal("locally blocked menu intent was rejected")
	}
	if evidence.StageFailureObserved() || evidence.BoundaryDropObserved() {
		t.Fatal("safe local menu block was classified as a failure")
	}
}

func TestSupervisorFailureEvidenceRequiresClassifiedTotal(t *testing.T) {
	descriptor := FailureDescriptorEvidence{Component: "source-capture", Phase: "capture-activate", Code: "activation_failed"}
	evidence := SupervisorEvidence{
		SessionFailures: 2,
		FirstFailure:    descriptor,
		FailureCounts:   []FailureCountEvidence{{FailureDescriptorEvidence: descriptor, Count: 2}},
	}
	if !evidence.FailureEvidenceValid() {
		t.Fatal("valid failure evidence was rejected")
	}
	evidence.FailureCounts[0].Count = 1
	if evidence.FailureEvidenceValid() {
		t.Fatal("incomplete failure total was accepted")
	}
	evidence.FailureCounts[0].Count = 2
	evidence.FirstFailure.Component = "private"
	if evidence.FailureEvidenceValid() {
		t.Fatal("private failure descriptor was accepted")
	}
	evidence.FirstFailure = descriptor
	evidence.FailureCounts = []FailureCountEvidence{
		{FailureDescriptorEvidence: descriptor, Count: 1},
		{FailureDescriptorEvidence: descriptor, Count: 1},
	}
	if evidence.FailureEvidenceValid() {
		t.Fatal("duplicate failure descriptors were accepted")
	}
}
