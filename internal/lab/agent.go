package lab

import "time"

const AgentSessionSchemaVersion = "krasis.lab.agent-session/v21"

type AgentSessionReport struct {
	SchemaVersion       string                      `json:"schemaVersion"`
	DeviceAlias         string                      `json:"deviceAlias"`
	Role                string                      `json:"role"`
	Edge                string                      `json:"edge"`
	CollectedAt         time.Time                   `json:"collectedAt"`
	BuildRevision       string                      `json:"buildRevision"`
	BuildModified       bool                        `json:"buildModified"`
	RequestedMillis     int64                       `json:"requestedDurationMillis"`
	HeartbeatMillis     int64                       `json:"heartbeatIntervalMillis"`
	PeerTimeoutMillis   int64                       `json:"peerTimeoutMillis"`
	Completed           bool                        `json:"completed"`
	Passed              bool                        `json:"passed"`
	StopReason          string                      `json:"stopReason"`
	FailureCode         string                      `json:"failureCode,omitempty"`
	Lifecycle           LifecycleEvidence           `json:"lifecycle"`
	CrashGuard          CrashGuardEvidence          `json:"crashGuard"`
	Supervisor          SupervisorEvidence          `json:"supervisor"`
	Liveness            LivenessEvidence            `json:"liveness"`
	SourceInput         SourceInputEvidence         `json:"sourceInput"`
	SourcePipeline      SourcePipelineEvidence      `json:"sourcePipeline"`
	ControlFlow         ControlFlowEvidence         `json:"controlFlow"`
	DestinationPipeline DestinationPipelineEvidence `json:"destinationPipeline"`
	DestinationInput    InputEvidence               `json:"destinationInput"`
}

type LifecycleEvidence struct {
	MonitorStarted bool   `json:"monitorStarted"`
	CleanShutdown  bool   `json:"cleanShutdown"`
	Event          string `json:"event,omitempty"`
	RearmRequired  bool   `json:"rearmRequired"`
}

type LivenessEvidence struct {
	HeartbeatsSent      uint64 `json:"heartbeatsSent"`
	HeartbeatsReceived  uint64 `json:"heartbeatsReceived"`
	ReceiveGapSamples   uint64 `json:"receiveGapSamples"`
	MaxReceiveGapMillis int64  `json:"maxReceiveGapMillis"`
}

type CrashGuardEvidence struct {
	Started       bool   `json:"started"`
	CleanShutdown bool   `json:"cleanShutdown"`
	ReportFile    string `json:"reportFile,omitempty"`
}

type SupervisorEvidence struct {
	FinalState      string                    `json:"finalState"`
	ActiveCount     uint64                    `json:"activeCount"`
	AcquireFailures uint64                    `json:"acquireFailures"`
	SessionFailures uint64                    `json:"sessionFailures"`
	EmergencyStops  uint64                    `json:"emergencyStops"`
	FirstFailure    FailureDescriptorEvidence `json:"firstFailure"`
	FailureCounts   []FailureCountEvidence    `json:"failureCounts"`
}

type FailureDescriptorEvidence struct {
	Component string `json:"component,omitempty"`
	Phase     string `json:"phase,omitempty"`
	Code      string `json:"code,omitempty"`
}

type FailureCountEvidence struct {
	FailureDescriptorEvidence
	Count uint64 `json:"count"`
}

func (evidence SupervisorEvidence) FailureEvidenceValid() bool {
	if evidence.SessionFailures == 0 {
		return evidence.FirstFailure == (FailureDescriptorEvidence{}) && len(evidence.FailureCounts) == 0
	}
	if !ValidFailureDescriptor(evidence.FirstFailure) || len(evidence.FailureCounts) == 0 {
		return false
	}
	total := uint64(0)
	firstFound := false
	seen := make(map[FailureDescriptorEvidence]struct{}, len(evidence.FailureCounts))
	for _, current := range evidence.FailureCounts {
		if current.Count == 0 || !ValidFailureDescriptor(current.FailureDescriptorEvidence) || total > ^uint64(0)-current.Count {
			return false
		}
		if _, duplicate := seen[current.FailureDescriptorEvidence]; duplicate {
			return false
		}
		seen[current.FailureDescriptorEvidence] = struct{}{}
		total += current.Count
		if current.FailureDescriptorEvidence == evidence.FirstFailure {
			firstFound = true
		}
	}
	return firstFound && total == evidence.SessionFailures
}

func ValidFailureDescriptor(descriptor FailureDescriptorEvidence) bool {
	key := descriptor.Component + "|" + descriptor.Phase + "|" + descriptor.Code
	switch key {
	case "source-capture|capture-activate|activation_failed",
		"source-capture|capture-activate|setup_failed",
		"source-capture|capture-activate|api_rejected",
		"source-capture|capture-activate|verification_lost",
		"source-capture|capture-activate|thread_attach_failed",
		"source-capture|capture-activate|thread_detach_failed",
		"source-capture|focus-restore|api_rejected",
		"source-capture|focus-restore|verification_lost",
		"source-capture|focus-restore|thread_attach_failed",
		"source-capture|focus-restore|thread_detach_failed",
		"source-capture|focus-release|capture_retained",
		"source-session|source-activate|activation_failed",
		"source-session|runtime|connection_interrupted",
		"outbound-control|grant-apply|state_transition_failed",
		"boundary-admission|request-begin|state_transition_failed",
		"peer-writer|request-write|outcome_unknown",
		"peer-writer|grant-write|outcome_unknown",
		"peer-writer|revoke-write|outcome_unknown",
		"peer-reader|read|after_local_interrupt",
		"peer-reader|read|remote_eof",
		"peer-reader|read|frame_truncated",
		"peer-reader|read|io_failure",
		"liveness|heartbeat|peer_timeout",
		"peer-dispatch|control|failed",
		"peer-dispatch|input|failed",
		"peer-dispatch|heartbeat|failed",
		"source-session|runtime|component_failed",
		"source-hook|runtime|component_failed",
		"boundary-orchestrator|runtime|component_failed",
		"return-controller|runtime|component_failed",
		"peer-session|runtime|component_failed",
		"peer-liveness|runtime|component_failed",
		"unknown|runtime|component_failed":
		return true
	default:
		return false
	}
}

type SourceInputEvidence struct {
	BoundaryIntents uint64 `json:"boundaryIntents"`
	KeyDowns        uint64 `json:"keyDowns"`
	KeyUps          uint64 `json:"keyUps"`
	ButtonDowns     uint64 `json:"buttonDowns"`
	ButtonUps       uint64 `json:"buttonUps"`
	PointerMoves    uint64 `json:"pointerMoves"`
	Wheels          uint64 `json:"wheels"`
}

func (evidence SourceInputEvidence) TotalActions() uint64 {
	total, valid := evidence.CheckedTotalActions()
	if !valid {
		return ^uint64(0)
	}
	return total
}

func (evidence SourceInputEvidence) CheckedTotalActions() (uint64, bool) {
	return checkedUint64Sum(
		evidence.KeyDowns, evidence.KeyUps, evidence.ButtonDowns,
		evidence.ButtonUps, evidence.PointerMoves, evidence.Wheels,
	)
}

type SourcePipelineEvidence struct {
	Wheel WheelPipelineEvidence `json:"wheel"`
	Queue SourceQueueEvidence   `json:"queue"`
}

type SourceQueueEvidence struct {
	Enqueued           uint64 `json:"enqueued"`
	Dequeued           uint64 `json:"dequeued"`
	DiscardedStale     uint64 `json:"discardedStale"`
	MaxDepth           uint64 `json:"maxDepth"`
	ResidenceSamples   uint64 `json:"residenceSamples"`
	MaxResidenceMicros int64  `json:"maxResidenceMicros"`
}

type ControlFlowEvidence struct {
	Boundary  BoundaryFlowEvidence  `json:"boundary"`
	Outbound  OutboundFlowEvidence  `json:"outbound"`
	Inbound   InboundFlowEvidence   `json:"inbound"`
	Transport TransportFlowEvidence `json:"transport"`
}

type BoundaryFlowEvidence struct {
	Queued                  uint64 `json:"queued"`
	Admitted                uint64 `json:"admitted"`
	DroppedNotReady         uint64 `json:"droppedNotReady"`
	BlockedLocalMenu        uint64 `json:"blockedLocalMenu"`
	BlockedLocalInteraction uint64 `json:"blockedLocalInteraction"`
	BlockedMouseCapture     uint64 `json:"blockedMouseCapture"`
	BlockedPressedInput     uint64 `json:"blockedPressedInput"`
	BlockedMoveSize         uint64 `json:"blockedMoveSize"`
	AdmissionFailed         uint64 `json:"admissionFailed"`
}

type OutboundFlowEvidence struct {
	RequestWriteCompleted             uint64 `json:"requestWriteCompleted"`
	RequestWriteOutcomeUnknown        uint64 `json:"requestWriteOutcomeUnknown"`
	GrantReceived                     uint64 `json:"grantReceived"`
	GrantApplySucceeded               uint64 `json:"grantApplySucceeded"`
	GrantApplyFailed                  uint64 `json:"grantApplyFailed"`
	SourceSessionActivationSucceeded  uint64 `json:"sourceSessionActivationSucceeded"`
	SourceSessionActivationFailed     uint64 `json:"sourceSessionActivationFailed"`
	CaptureActivationAttempted        uint64 `json:"captureActivationAttempted"`
	CaptureActivationSucceeded        uint64 `json:"captureActivationSucceeded"`
	CaptureActivationFailed           uint64 `json:"captureActivationFailed"`
	CaptureActivationSetupFailed      uint64 `json:"captureActivationSetupFailed"`
	CaptureActivationAPIRejected      uint64 `json:"captureActivationApiRejected"`
	CaptureActivationVerificationLost uint64 `json:"captureActivationVerificationLost"`
	CommitWriteCompleted              uint64 `json:"commitWriteCompleted"`
	CommitWriteOutcomeUnknown         uint64 `json:"commitWriteOutcomeUnknown"`
	AbortWriteCompleted               uint64 `json:"abortWriteCompleted"`
	AbortWriteOutcomeUnknown          uint64 `json:"abortWriteOutcomeUnknown"`
	RevokeReceived                    uint64 `json:"revokeReceived"`
}

type InboundFlowEvidence struct {
	RequestReceived           uint64 `json:"requestReceived"`
	ReturnArmSucceeded        uint64 `json:"returnArmSucceeded"`
	EntryPrepareAttempted     uint64 `json:"entryPrepareAttempted"`
	EntryPrepared             uint64 `json:"entryPrepared"`
	GrantWriteCompleted       uint64 `json:"grantWriteCompleted"`
	GrantWriteOutcomeUnknown  uint64 `json:"grantWriteOutcomeUnknown"`
	CommitReceived            uint64 `json:"commitReceived"`
	AbortReceived             uint64 `json:"abortReceived"`
	EntryCommitted            uint64 `json:"entryCommitted"`
	EntryRollbackAttempted    uint64 `json:"entryRollbackAttempted"`
	RevokeWriteCompleted      uint64 `json:"revokeWriteCompleted"`
	RevokeWriteOutcomeUnknown uint64 `json:"revokeWriteOutcomeUnknown"`
}

type TransportFlowEvidence struct {
	LocalConnectionInterrupts uint64 `json:"localConnectionInterrupts"`
	ReadAfterLocalInterrupt   uint64 `json:"readAfterLocalInterrupt"`
}

func (evidence ControlFlowEvidence) ValidAgainst(
	source SourceInputEvidence,
	wheel WheelPipelineEvidence,
	destination InputEvidence,
) bool {
	sourceActions, sourceActionsValid := source.CheckedTotalActions()
	boundaryClassified, boundaryValid := checkedUint64Sum(
		evidence.Boundary.Admitted,
		evidence.Boundary.DroppedNotReady,
		evidence.Boundary.BlockedLocalMenu,
		evidence.Boundary.BlockedLocalInteraction,
		evidence.Boundary.AdmissionFailed,
	)
	localInteractionClassified, localInteractionValid := checkedUint64Sum(
		evidence.Boundary.BlockedMouseCapture,
		evidence.Boundary.BlockedPressedInput,
		evidence.Boundary.BlockedMoveSize,
	)
	grantApplyClassified, grantApplyValid := checkedUint64Sum(
		evidence.Outbound.GrantApplySucceeded,
		evidence.Outbound.GrantApplyFailed,
	)
	sourceActivationClassified, sourceActivationValid := checkedUint64Sum(
		evidence.Outbound.SourceSessionActivationSucceeded,
		evidence.Outbound.SourceSessionActivationFailed,
	)
	captureActivationClassified, captureActivationValid := checkedUint64Sum(
		evidence.Outbound.CaptureActivationSucceeded,
		evidence.Outbound.CaptureActivationFailed,
	)
	captureFailureClassified, captureFailureValid := checkedUint64Sum(
		evidence.Outbound.CaptureActivationSetupFailed,
		evidence.Outbound.CaptureActivationAPIRejected,
		evidence.Outbound.CaptureActivationVerificationLost,
	)
	fallbackFailures, fallbackFailuresValid := checkedUint64Sum(
		wheel.ForegroundFallbackAttachFailed,
		wheel.ForegroundFallbackDetachFailed,
		wheel.ForegroundFallbackAPIRejected,
		wheel.ForegroundFallbackVerificationLost,
	)
	commitWriteClassified, commitWriteValid := checkedUint64Sum(
		evidence.Outbound.CommitWriteCompleted,
		evidence.Outbound.CommitWriteOutcomeUnknown,
	)
	abortWriteClassified, abortWriteValid := checkedUint64Sum(
		evidence.Outbound.AbortWriteCompleted,
		evidence.Outbound.AbortWriteOutcomeUnknown,
	)
	preCommitActivationFailures, preCommitActivationFailuresValid := checkedUint64Sum(
		evidence.Outbound.SourceSessionActivationFailed,
		evidence.Outbound.CaptureActivationFailed,
	)
	inboundPrepareTerminal, inboundPrepareTerminalValid := checkedUint64Sum(
		evidence.Inbound.EntryCommitted,
		evidence.Inbound.EntryRollbackAttempted,
	)
	return sourceActionsValid && boundaryValid && localInteractionValid &&
		localInteractionClassified == evidence.Boundary.BlockedLocalInteraction &&
		boundaryClassified <= evidence.Boundary.Queued &&
		(sourceActions == 0 || boundaryClassified == evidence.Boundary.Queued) &&
		evidence.Boundary.Queued == source.BoundaryIntents &&
		evidence.Boundary.Admitted == evidence.Outbound.RequestWriteCompleted &&
		grantApplyValid && grantApplyClassified == evidence.Outbound.GrantReceived &&
		sourceActivationValid && sourceActivationClassified == evidence.Outbound.GrantApplySucceeded &&
		evidence.Outbound.CaptureActivationAttempted == evidence.Outbound.SourceSessionActivationSucceeded &&
		captureActivationValid && captureActivationClassified == evidence.Outbound.CaptureActivationAttempted &&
		captureFailureValid && captureFailureClassified == evidence.Outbound.CaptureActivationFailed &&
		fallbackFailuresValid && wheel.ForegroundFallbackAttempted <= evidence.Outbound.CaptureActivationAttempted &&
		wheel.ForegroundFallbackSucceeded <= evidence.Outbound.CaptureActivationSucceeded &&
		fallbackFailures <= evidence.Outbound.CaptureActivationFailed &&
		commitWriteValid && commitWriteClassified == evidence.Outbound.CaptureActivationSucceeded &&
		abortWriteValid && preCommitActivationFailuresValid && abortWriteClassified == preCommitActivationFailures &&
		evidence.Outbound.CaptureActivationSucceeded == wheel.CaptureActivations &&
		evidence.Inbound.ReturnArmSucceeded == destination.ReturnArms &&
		evidence.Inbound.EntryPrepared <= evidence.Inbound.EntryPrepareAttempted &&
		evidence.Inbound.GrantWriteCompleted <= evidence.Inbound.EntryPrepared &&
		inboundPrepareTerminalValid && inboundPrepareTerminal == evidence.Inbound.EntryPrepareAttempted &&
		evidence.Inbound.CommitReceived == evidence.Inbound.EntryCommitted &&
		evidence.Inbound.EntryCommitted == destination.Entries &&
		evidence.Transport.ReadAfterLocalInterrupt <= evidence.Transport.LocalConnectionInterrupts
}

func (evidence ControlFlowEvidence) StageFailureObserved() bool {
	return evidence.Boundary.AdmissionFailed > 0 ||
		evidence.Outbound.RequestWriteOutcomeUnknown > 0 ||
		evidence.Outbound.GrantApplyFailed > 0 ||
		evidence.Outbound.SourceSessionActivationFailed > 0 ||
		evidence.Outbound.CaptureActivationFailed > 0 ||
		evidence.Outbound.CommitWriteOutcomeUnknown > 0 ||
		evidence.Outbound.AbortWriteOutcomeUnknown > 0 ||
		evidence.Inbound.GrantWriteOutcomeUnknown > 0 ||
		evidence.Inbound.RevokeWriteOutcomeUnknown > 0
}

func (evidence ControlFlowEvidence) BoundaryDropObserved() bool {
	return evidence.Boundary.DroppedNotReady > 0
}

type WheelPipelineEvidence struct {
	HookObserved                         uint64 `json:"hookObserved"`
	SynthesizedObserved                  uint64 `json:"synthesizedObserved"`
	RawObserved                          uint64 `json:"rawObserved"`
	WindowObserved                       uint64 `json:"windowObserved"`
	ActiveHookMoves                      uint64 `json:"activeHookMoves"`
	ActiveHookButtons                    uint64 `json:"activeHookButtons"`
	CaptureActivations                   uint64 `json:"captureActivations"`
	CaptureDeactivations                 uint64 `json:"captureDeactivations"`
	ForegroundFallbackAttempted          uint64 `json:"foregroundFallbackAttempted"`
	ForegroundFallbackSucceeded          uint64 `json:"foregroundFallbackSucceeded"`
	ForegroundFallbackAttachFailed       uint64 `json:"foregroundFallbackAttachFailed"`
	ForegroundFallbackDetachFailed       uint64 `json:"foregroundFallbackDetachFailed"`
	ForegroundFallbackAPIRejected        uint64 `json:"foregroundFallbackApiRejected"`
	ForegroundFallbackVerificationLost   uint64 `json:"foregroundFallbackVerificationLost"`
	FocusRestoreFallbackAttempted        uint64 `json:"focusRestoreFallbackAttempted"`
	FocusRestoreFallbackSucceeded        uint64 `json:"focusRestoreFallbackSucceeded"`
	FocusRestoreFallbackAttachFailed     uint64 `json:"focusRestoreFallbackAttachFailed"`
	FocusRestoreFallbackDetachFailed     uint64 `json:"focusRestoreFallbackDetachFailed"`
	FocusRestoreFallbackAPIRejected      uint64 `json:"focusRestoreFallbackApiRejected"`
	FocusRestoreFallbackVerificationLost uint64 `json:"focusRestoreFallbackVerificationLost"`
	FocusRestoreSkipped                  uint64 `json:"focusRestoreSkipped"`
	FocusRestored                        uint64 `json:"focusRestored"`
	FocusRestoreFailed                   uint64 `json:"focusRestoreFailed"`
	FocusRestoreAPIRejected              uint64 `json:"focusRestoreApiRejected"`
	FocusRestoreVerificationLost         uint64 `json:"focusRestoreVerificationLost"`
	PostHideFocusAttempted               uint64 `json:"postHideFocusAttempted"`
	PostHideFocusRecovered               uint64 `json:"postHideFocusRecovered"`
	PostHideFocusUnavailable             uint64 `json:"postHideFocusUnavailable"`
	PostHideFocusCaptureRetained         uint64 `json:"postHideFocusCaptureRetained"`
	Mapped                               uint64 `json:"mapped"`
	QueueAccepted                        uint64 `json:"queueAccepted"`
	QueueRejected                        uint64 `json:"queueRejected"`
	RejectedInactive                     uint64 `json:"rejectedInactive"`
	RejectedInvalid                      uint64 `json:"rejectedInvalid"`
	RejectedOverflow                     uint64 `json:"rejectedOverflow"`
	Sent                                 uint64 `json:"sent"`
}

func (evidence WheelPipelineEvidence) RawWheelUnforwarded() bool {
	return evidence.RawObserved > 0 && evidence.Sent == 0
}

func (evidence WheelPipelineEvidence) WindowWheelUnforwarded() bool {
	return evidence.WindowObserved > 0 && evidence.Sent == 0
}

func (evidence WheelPipelineEvidence) CaptureLifecycleSafe() bool {
	fallbackOutcomes, fallbackValid := checkedUint64Sum(
		evidence.ForegroundFallbackSucceeded,
		evidence.ForegroundFallbackAttachFailed,
		evidence.ForegroundFallbackDetachFailed,
		evidence.ForegroundFallbackAPIRejected,
		evidence.ForegroundFallbackVerificationLost,
	)
	if !fallbackValid || fallbackOutcomes != evidence.ForegroundFallbackAttempted ||
		evidence.ForegroundFallbackSucceeded > evidence.CaptureActivations {
		return false
	}
	focusFallbackOutcomes, focusFallbackValid := checkedUint64Sum(
		evidence.FocusRestoreFallbackSucceeded,
		evidence.FocusRestoreFallbackAttachFailed,
		evidence.FocusRestoreFallbackDetachFailed,
		evidence.FocusRestoreFallbackAPIRejected,
		evidence.FocusRestoreFallbackVerificationLost,
	)
	if !focusFallbackValid || focusFallbackOutcomes != evidence.FocusRestoreFallbackAttempted ||
		evidence.FocusRestoreFallbackSucceeded > evidence.FocusRestored {
		return false
	}
	focusFallbackFailures, focusFallbackFailuresValid := checkedUint64Sum(
		evidence.FocusRestoreFallbackAttachFailed,
		evidence.FocusRestoreFallbackDetachFailed,
		evidence.FocusRestoreFallbackAPIRejected,
		evidence.FocusRestoreFallbackVerificationLost,
	)
	if !focusFallbackFailuresValid || focusFallbackFailures != evidence.FocusRestoreFailed {
		return false
	}
	focusFailures, failuresValid := checkedUint64Sum(
		evidence.FocusRestoreAPIRejected,
		evidence.FocusRestoreVerificationLost,
	)
	if !failuresValid || focusFailures != evidence.FocusRestoreFailed {
		return false
	}
	postHideOutcomes, postHideValid := checkedUint64Sum(
		evidence.PostHideFocusRecovered,
		evidence.PostHideFocusUnavailable,
		evidence.PostHideFocusCaptureRetained,
	)
	if !postHideValid || postHideOutcomes != evidence.PostHideFocusAttempted ||
		evidence.PostHideFocusAttempted != evidence.FocusRestoreFailed ||
		evidence.PostHideFocusCaptureRetained > 0 || evidence.FocusRestoreFallbackDetachFailed > 0 {
		return false
	}
	restores, restoresValid := checkedUint64Sum(
		evidence.FocusRestoreSkipped,
		evidence.FocusRestored,
		evidence.FocusRestoreFailed,
	)
	if !restoresValid {
		return false
	}
	return evidence.CaptureDeactivations == evidence.CaptureActivations &&
		restores == evidence.CaptureDeactivations
}

func (evidence SourcePipelineEvidence) ValidAgainst(source SourceInputEvidence) bool {
	wheel := evidence.Wheel
	queue := evidence.Queue
	sourceActions, sourceActionsValid := source.CheckedTotalActions()
	queueValid := queue.Dequeued <= queue.Enqueued && queue.DiscardedStale <= queue.Dequeued &&
		queue.ResidenceSamples == queue.Dequeued && queue.MaxResidenceMicros >= 0 &&
		((queue.Enqueued == 0 && queue.MaxDepth == 0) || (queue.Enqueued > 0 && queue.MaxDepth > 0 && queue.MaxDepth <= queue.Enqueued)) &&
		sourceActions <= queue.Dequeued-queue.DiscardedStale
	return sourceActionsValid && queueValid && (sourceActions == 0 || wheel.CaptureActivations > 0) &&
		wheel.SynthesizedObserved <= ^uint64(0)-wheel.HookObserved &&
		wheel.WindowObserved <= ^uint64(0)-wheel.HookObserved-wheel.SynthesizedObserved &&
		wheel.Mapped <= wheel.HookObserved+wheel.SynthesizedObserved+wheel.WindowObserved && wheel.QueueAccepted <= wheel.Mapped &&
		wheel.QueueRejected == wheel.Mapped-wheel.QueueAccepted &&
		wheel.RejectedInactive <= wheel.QueueRejected &&
		wheel.RejectedInvalid <= wheel.QueueRejected-wheel.RejectedInactive &&
		wheel.RejectedOverflow == wheel.QueueRejected-wheel.RejectedInactive-wheel.RejectedInvalid &&
		wheel.Sent <= wheel.QueueAccepted && wheel.Sent == source.Wheels && wheel.CaptureLifecycleSafe()
}

type DestinationPipelineEvidence struct {
	Received       uint64 `json:"received"`
	Applied        uint64 `json:"applied"`
	RetiredDrained uint64 `json:"retiredDrained"`
	Rejected       uint64 `json:"rejected"`
}

func (evidence DestinationPipelineEvidence) ValidAgainst(input InputEvidence) bool {
	outcomes, valid := checkedUint64Sum(evidence.Applied, evidence.RetiredDrained, evidence.Rejected)
	inputActions, inputValid := input.CheckedTotalActions()
	return valid && inputValid && outcomes == evidence.Received && evidence.Applied == inputActions
}

type InputEvidence struct {
	Entries           uint64                   `json:"entries"`
	KeyDowns          uint64                   `json:"keyDowns"`
	KeyUps            uint64                   `json:"keyUps"`
	ButtonDowns       uint64                   `json:"buttonDowns"`
	ButtonUps         uint64                   `json:"buttonUps"`
	PointerMoves      uint64                   `json:"pointerMoves"`
	Wheels            uint64                   `json:"wheels"`
	ReturnArms        uint64                   `json:"returnArms"`
	InjectionFailures InjectionFailureEvidence `json:"injectionFailures"`
	CursorRollbacks   CursorRollbackEvidence   `json:"cursorRollbacks"`
}

func (evidence InputEvidence) TotalActions() uint64 {
	total, valid := evidence.CheckedTotalActions()
	if !valid {
		return ^uint64(0)
	}
	return total
}

func (evidence InputEvidence) CheckedTotalActions() (uint64, bool) {
	return checkedUint64Sum(
		evidence.KeyDowns, evidence.KeyUps, evidence.ButtonDowns,
		evidence.ButtonUps, evidence.PointerMoves, evidence.Wheels,
	)
}

type InjectionFailureEvidence struct {
	NotApplied       uint64 `json:"notApplied"`
	PartiallyApplied uint64 `json:"partiallyApplied"`
	OutcomeUnknown   uint64 `json:"outcomeUnknown"`
	Unclassified     uint64 `json:"unclassified"`
}

func (evidence InjectionFailureEvidence) Total() uint64 {
	total, valid := evidence.CheckedTotal()
	if !valid {
		return ^uint64(0)
	}
	return total
}

func (evidence InjectionFailureEvidence) CheckedTotal() (uint64, bool) {
	return checkedUint64Sum(
		evidence.NotApplied, evidence.PartiallyApplied,
		evidence.OutcomeUnknown, evidence.Unclassified,
	)
}

type CursorRollbackEvidence struct {
	Attempted   uint64 `json:"attempted"`
	Restored    uint64 `json:"restored"`
	NotRestored uint64 `json:"notRestored"`
	Failed      uint64 `json:"failed"`
}

func (evidence CursorRollbackEvidence) Classified() uint64 {
	total, valid := evidence.CheckedClassified()
	if !valid {
		return ^uint64(0)
	}
	return total
}

func (evidence CursorRollbackEvidence) CheckedClassified() (uint64, bool) {
	return checkedUint64Sum(evidence.Restored, evidence.NotRestored, evidence.Failed)
}

func checkedUint64Sum(values ...uint64) (uint64, bool) {
	var total uint64
	for _, value := range values {
		if value > ^uint64(0)-total {
			return 0, false
		}
		total += value
	}
	return total, true
}
