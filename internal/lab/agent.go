package lab

import "time"

const AgentSessionSchemaVersion = "krasis.lab.agent-session/v13"

type AgentSessionReport struct {
	SchemaVersion     string                 `json:"schemaVersion"`
	DeviceAlias       string                 `json:"deviceAlias"`
	Role              string                 `json:"role"`
	Edge              string                 `json:"edge"`
	CollectedAt       time.Time              `json:"collectedAt"`
	BuildRevision     string                 `json:"buildRevision"`
	BuildModified     bool                   `json:"buildModified"`
	RequestedMillis   int64                  `json:"requestedDurationMillis"`
	HeartbeatMillis   int64                  `json:"heartbeatIntervalMillis"`
	PeerTimeoutMillis int64                  `json:"peerTimeoutMillis"`
	Completed         bool                   `json:"completed"`
	Passed            bool                   `json:"passed"`
	StopReason        string                 `json:"stopReason"`
	FailureCode       string                 `json:"failureCode,omitempty"`
	Lifecycle         LifecycleEvidence      `json:"lifecycle"`
	CrashGuard        CrashGuardEvidence     `json:"crashGuard"`
	Supervisor        SupervisorEvidence     `json:"supervisor"`
	Liveness          LivenessEvidence       `json:"liveness"`
	SourceInput       SourceInputEvidence    `json:"sourceInput"`
	SourcePipeline    SourcePipelineEvidence `json:"sourcePipeline"`
	DestinationInput  InputEvidence          `json:"destinationInput"`
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
	FinalState           string `json:"finalState"`
	ActiveCount          uint64 `json:"activeCount"`
	AcquireFailures      uint64 `json:"acquireFailures"`
	SessionFailures      uint64 `json:"sessionFailures"`
	EmergencyStops       uint64 `json:"emergencyStops"`
	LastFailureComponent string `json:"lastFailureComponent,omitempty"`
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
}

type WheelPipelineEvidence struct {
	HookObserved         uint64 `json:"hookObserved"`
	SynthesizedObserved  uint64 `json:"synthesizedObserved"`
	RawObserved          uint64 `json:"rawObserved"`
	WindowObserved       uint64 `json:"windowObserved"`
	ActiveHookMoves      uint64 `json:"activeHookMoves"`
	ActiveHookButtons    uint64 `json:"activeHookButtons"`
	CaptureActivations   uint64 `json:"captureActivations"`
	CaptureDeactivations uint64 `json:"captureDeactivations"`
	FocusRestoreSkipped  uint64 `json:"focusRestoreSkipped"`
	FocusRestored        uint64 `json:"focusRestored"`
	FocusRestoreFailed   uint64 `json:"focusRestoreFailed"`
	Mapped               uint64 `json:"mapped"`
	QueueAccepted        uint64 `json:"queueAccepted"`
	QueueRejected        uint64 `json:"queueRejected"`
	RejectedInactive     uint64 `json:"rejectedInactive"`
	RejectedInvalid      uint64 `json:"rejectedInvalid"`
	RejectedOverflow     uint64 `json:"rejectedOverflow"`
	Sent                 uint64 `json:"sent"`
}

func (evidence WheelPipelineEvidence) RawWheelUnforwarded() bool {
	return evidence.RawObserved > 0 && evidence.Sent == 0
}

func (evidence WheelPipelineEvidence) WindowWheelUnforwarded() bool {
	return evidence.WindowObserved > 0 && evidence.Sent == 0
}

func (evidence WheelPipelineEvidence) CaptureLifecycleSafe() bool {
	if evidence.FocusRestored > ^uint64(0)-evidence.FocusRestoreSkipped ||
		evidence.FocusRestoreFailed > ^uint64(0)-evidence.FocusRestoreSkipped-evidence.FocusRestored {
		return false
	}
	restores := evidence.FocusRestoreSkipped + evidence.FocusRestored + evidence.FocusRestoreFailed
	return evidence.CaptureDeactivations == evidence.CaptureActivations &&
		restores == evidence.CaptureDeactivations && evidence.FocusRestoreFailed == 0
}

func (evidence SourcePipelineEvidence) ValidAgainst(source SourceInputEvidence) bool {
	wheel := evidence.Wheel
	sourceActions, sourceActionsValid := source.CheckedTotalActions()
	return sourceActionsValid && (sourceActions == 0 || wheel.CaptureActivations > 0) &&
		wheel.SynthesizedObserved <= ^uint64(0)-wheel.HookObserved &&
		wheel.WindowObserved <= ^uint64(0)-wheel.HookObserved-wheel.SynthesizedObserved &&
		wheel.Mapped <= wheel.HookObserved+wheel.SynthesizedObserved+wheel.WindowObserved && wheel.QueueAccepted <= wheel.Mapped &&
		wheel.QueueRejected == wheel.Mapped-wheel.QueueAccepted &&
		wheel.RejectedInactive <= wheel.QueueRejected &&
		wheel.RejectedInvalid <= wheel.QueueRejected-wheel.RejectedInactive &&
		wheel.RejectedOverflow == wheel.QueueRejected-wheel.RejectedInactive-wheel.RejectedInvalid &&
		wheel.Sent <= wheel.QueueAccepted && wheel.Sent == source.Wheels && wheel.CaptureLifecycleSafe()
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
