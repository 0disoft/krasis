package lab

import (
	"context"
	"errors"
	"time"
)

const (
	DiagnoseSchemaVersion        = "krasis.lab.diagnose/v1"
	CaptureSchemaVersion         = "krasis.lab.capture/v1"
	SelfTestSchemaVersion        = "krasis.lab.selftest-inject/v1"
	StressSchemaVersion          = "krasis.lab.stress-selftest/v1"
	ProbeSchemaVersion           = "krasis.lab.probe/v1"
	GuidedProbeSchemaVersion     = "krasis.lab.guided-probe/v1"
	PointerShadowSchemaVersion   = "krasis.lab.pointer-shadow/v1"
	WheelSurfaceSchemaVersion    = "krasis.lab.wheel-surface/v1"
	TouchpadSurfaceSchemaVersion = "krasis.lab.touchpad-surface/v1"
)

var ErrUnsupported = errors.New("operation is not supported on this platform")

func ValidProbeDeviceAlias(alias string) bool {
	switch alias {
	case "H-A", "H-B", "S-A", "S-B":
		return true
	default:
		return false
	}
}

type Platform interface {
	Diagnose(context.Context) (Diagnostics, error)
	Capture(context.Context, CaptureOptions, func(CaptureSnapshot)) error
	SelfTestInject(context.Context) (SelfTestResult, error)
}

type PointerShadowPlatform interface {
	PointerShadow(context.Context, PointerShadowOptions) (PointerShadowObservation, error)
}

type WheelSurfacePlatform interface {
	WheelSurface(context.Context, WheelSurfaceOptions) (WheelSurfaceObservation, error)
}

type TouchpadSurfacePlatform interface {
	TouchpadSurface(context.Context, TouchpadSurfaceOptions) (TouchpadSurfaceObservation, error)
}

type TouchpadSurfaceOptions struct {
	Duration time.Duration
	Ready    func()
}

type TouchpadSurfaceObservation struct {
	ElapsedMillis int64                         `json:"elapsedMillis"`
	StopReason    string                        `json:"stopReason"`
	API           TouchpadSurfaceAPIEvidence    `json:"api"`
	Window        TouchpadSurfaceWindowEvidence `json:"window"`
	Pointer       TouchpadSurfacePointerCounts  `json:"pointer"`
	Legacy        TouchpadSurfaceLegacyCounts   `json:"legacy"`
}

type TouchpadSurfaceAPIEvidence struct {
	RegisterAvailable    bool `json:"registerAvailable"`
	PointerTypeAvailable bool `json:"pointerTypeAvailable"`
	Registered           bool `json:"registered"`
}

type TouchpadSurfaceWindowEvidence struct {
	Created            bool   `json:"created"`
	ForegroundObserved bool   `json:"foregroundObserved"`
	Activations        uint64 `json:"activations"`
	Deactivations      uint64 `json:"deactivations"`
}

type TouchpadSurfacePointerCounts struct {
	Down              uint64 `json:"down"`
	Update            uint64 `json:"update"`
	Up                uint64 `json:"up"`
	Enter             uint64 `json:"enter"`
	Leave             uint64 `json:"leave"`
	CaptureChanged    uint64 `json:"captureChanged"`
	Wheel             uint64 `json:"wheel"`
	HorizontalWheel   uint64 `json:"horizontalWheel"`
	TypeQueries       uint64 `json:"typeQueries"`
	TypeQueryFailures uint64 `json:"typeQueryFailures"`
	TouchpadConfirmed uint64 `json:"touchpadConfirmed"`
	OtherConfirmed    uint64 `json:"otherConfirmed"`
}

type TouchpadSurfaceLegacyCounts struct {
	VerticalWheels   uint64 `json:"verticalWheels"`
	HorizontalWheels uint64 `json:"horizontalWheels"`
}

type TouchpadSurfaceSignature struct {
	TouchpadPointerObserved bool   `json:"touchpadPointerObserved"`
	LegacyWheelObserved     bool   `json:"legacyWheelObserved"`
	ObservedPath            string `json:"observedPath"`
}

type TouchpadSurfaceReport struct {
	SchemaVersion       string                      `json:"schemaVersion"`
	DeviceAlias         string                      `json:"deviceAlias"`
	CollectedAt         time.Time                   `json:"collectedAt"`
	BuildRevision       string                      `json:"buildRevision"`
	BuildModified       bool                        `json:"buildModified"`
	RequestedDurationMs int64                       `json:"requestedDurationMillis"`
	Completed           bool                        `json:"completed"`
	Passed              bool                        `json:"passed"`
	FailureCodes        []string                    `json:"failureCodes"`
	Diagnostics         *Diagnostics                `json:"diagnostics,omitempty"`
	Observation         *TouchpadSurfaceObservation `json:"observation,omitempty"`
	Signature           TouchpadSurfaceSignature    `json:"signature"`
}

type WheelSurfaceOptions struct {
	Duration time.Duration
	Ready    func()
}

type WheelSurfaceObservation struct {
	ElapsedMillis int64                      `json:"elapsedMillis"`
	Hook          WheelSurfaceHookCounts     `json:"hook"`
	Raw           WheelSurfaceRawCounts      `json:"raw"`
	Sentinel      WheelSurfaceSentinelCounts `json:"sentinel"`
}

type WheelSurfaceHookCounts struct {
	PhysicalMoves               uint64 `json:"physicalMoves"`
	PhysicalButtonDowns         uint64 `json:"physicalButtonDowns"`
	PhysicalButtonUps           uint64 `json:"physicalButtonUps"`
	PhysicalVerticalWheels      uint64 `json:"physicalVerticalWheels"`
	PhysicalHorizontalWheels    uint64 `json:"physicalHorizontalWheels"`
	SynthesizedVerticalWheels   uint64 `json:"synthesizedVerticalWheels"`
	SynthesizedHorizontalWheels uint64 `json:"synthesizedHorizontalWheels"`
}

type WheelSurfaceRawCounts struct {
	Packets                  uint64 `json:"packets"`
	ZeroDevicePackets        uint64 `json:"zeroDevicePackets"`
	TaggedInjectedPackets    uint64 `json:"taggedInjectedPackets"`
	VerticalWheelPackets     uint64 `json:"verticalWheelPackets"`
	HorizontalWheelPackets   uint64 `json:"horizontalWheelPackets"`
	VerticalDeltaTotal       int64  `json:"verticalDeltaTotal"`
	HorizontalDeltaTotal     int64  `json:"horizontalDeltaTotal"`
	VerticalDeltaMagnitude   uint64 `json:"verticalDeltaMagnitude"`
	HorizontalDeltaMagnitude uint64 `json:"horizontalDeltaMagnitude"`
}

type WheelSurfaceSentinelCounts struct {
	Requested uint64 `json:"requested"`
	Inserted  uint64 `json:"inserted"`
	Observed  uint64 `json:"observed"`
	Swallowed uint64 `json:"swallowed"`
}

type WheelSurfaceSignature struct {
	HookHealthConfirmed          bool   `json:"hookHealthConfirmed"`
	PhysicalInteractionObserved  bool   `json:"physicalInteractionObserved"`
	HookPhysicalWheelObserved    bool   `json:"hookPhysicalWheelObserved"`
	HookSynthesizedWheelObserved bool   `json:"hookSynthesizedWheelObserved"`
	HookWheelObserved            bool   `json:"hookWheelObserved"`
	RawWheelObserved             bool   `json:"rawWheelObserved"`
	ObservedPath                 string `json:"observedPath"`
}

type WheelSurfaceReport struct {
	SchemaVersion       string                   `json:"schemaVersion"`
	DeviceAlias         string                   `json:"deviceAlias"`
	CollectedAt         time.Time                `json:"collectedAt"`
	BuildRevision       string                   `json:"buildRevision"`
	BuildModified       bool                     `json:"buildModified"`
	RequestedDurationMs int64                    `json:"requestedDurationMillis"`
	Completed           bool                     `json:"completed"`
	Passed              bool                     `json:"passed"`
	FailureCodes        []string                 `json:"failureCodes"`
	Diagnostics         *Diagnostics             `json:"diagnostics,omitempty"`
	Observation         *WheelSurfaceObservation `json:"observation,omitempty"`
	Signature           WheelSurfaceSignature    `json:"signature"`
}

type PointerShadowOptions struct {
	Duration time.Duration
	Edge     string
	Ready    func()
}

type PointerShadowObservation struct {
	ElapsedMillis int64                   `json:"elapsedMillis"`
	Raw           PointerShadowRawCounts  `json:"raw"`
	Hook          PointerShadowHookCounts `json:"hook"`
}

type PointerShadowRawCounts struct {
	RelativePackets        uint64 `json:"relativePackets"`
	AbsolutePackets        uint64 `json:"absolutePackets"`
	ZeroMovementPackets    uint64 `json:"zeroMovementPackets"`
	OutwardPackets         uint64 `json:"outwardPackets"`
	InwardPackets          uint64 `json:"inwardPackets"`
	OutwardWhileHookAtEdge uint64 `json:"outwardWhileHookAtEdge"`
	ZeroDevicePackets      uint64 `json:"zeroDevicePackets"`
}

type PointerShadowHookCounts struct {
	PhysicalMoves        uint64 `json:"physicalMoves"`
	InjectedMoves        uint64 `json:"injectedMoves"`
	EdgeMoves            uint64 `json:"edgeMoves"`
	UnchangedAtEdgeMoves uint64 `json:"unchangedAtEdgeMoves"`
	OutwardAtEdgeMoves   uint64 `json:"outwardAtEdgeMoves"`
	InwardAtEdgeMoves    uint64 `json:"inwardAtEdgeMoves"`
}

type PointerShadowReport struct {
	SchemaVersion       string                    `json:"schemaVersion"`
	DeviceAlias         string                    `json:"deviceAlias"`
	Edge                string                    `json:"edge"`
	CollectedAt         time.Time                 `json:"collectedAt"`
	BuildRevision       string                    `json:"buildRevision"`
	BuildModified       bool                      `json:"buildModified"`
	RequestedDurationMs int64                     `json:"requestedDurationMillis"`
	Completed           bool                      `json:"completed"`
	Passed              bool                      `json:"passed"`
	FailureCodes        []string                  `json:"failureCodes"`
	Diagnostics         *Diagnostics              `json:"diagnostics,omitempty"`
	Observation         *PointerShadowObservation `json:"observation,omitempty"`
	Signature           PointerShadowSignature    `json:"signature"`
}

type PointerShadowSignature struct {
	RawOutwardAtEdge       bool `json:"rawOutwardAtEdge"`
	LegacyXUnchangedAtEdge bool `json:"legacyXUnchangedAtEdge"`
	LegacyOutwardDeficit   bool `json:"legacyOutwardDeficit"`
	Observed               bool `json:"observed"`
}

type Diagnostics struct {
	SchemaVersion string       `json:"schemaVersion"`
	OS            string       `json:"os"`
	Architecture  string       `json:"architecture"`
	Windows       WindowsInfo  `json:"windows"`
	Process       ProcessInfo  `json:"process"`
	Display       DisplayInfo  `json:"display"`
	Capabilities  Capabilities `json:"capabilities"`
}

type WindowsInfo struct {
	Major uint32 `json:"major"`
	Minor uint32 `json:"minor"`
	Build uint32 `json:"build"`
}

type ProcessInfo struct {
	SessionID uint32 `json:"sessionId"`
	Elevated  bool   `json:"elevated"`
}

type DisplayInfo struct {
	MonitorCount int32 `json:"monitorCount"`
	Left         int32 `json:"left"`
	Top          int32 `json:"top"`
	Width        int32 `json:"width"`
	Height       int32 `json:"height"`
	MouseButtons int32 `json:"mouseButtons"`
	SwapButtons  bool  `json:"swapButtons"`
}

type Capabilities struct {
	LowLevelKeyboardHook bool `json:"lowLevelKeyboardHook"`
	LowLevelMouseHook    bool `json:"lowLevelMouseHook"`
	RawInput             bool `json:"rawInput"`
	SendInput            bool `json:"sendInput"`
}

type CaptureOptions struct {
	ReportInterval time.Duration
	Ready          func()
}

type CaptureSnapshot struct {
	SchemaVersion string        `json:"schemaVersion"`
	Elapsed       time.Duration `json:"-"`
	ElapsedMillis int64         `json:"elapsedMillis"`
	Keyboard      EventCounts   `json:"keyboard"`
	Mouse         MouseCounts   `json:"mouse"`
}

type EventCounts struct {
	PhysicalDown uint64 `json:"physicalDown"`
	PhysicalUp   uint64 `json:"physicalUp"`
	InjectedDown uint64 `json:"injectedDown"`
	InjectedUp   uint64 `json:"injectedUp"`
	Other        uint64 `json:"other"`
}

type MouseCounts struct {
	PhysicalMove         uint64 `json:"physicalMove"`
	InjectedMove         uint64 `json:"injectedMove"`
	PhysicalButtonDown   uint64 `json:"physicalButtonDown"`
	PhysicalButtonUp     uint64 `json:"physicalButtonUp"`
	InjectedButtonDown   uint64 `json:"injectedButtonDown"`
	InjectedButtonUp     uint64 `json:"injectedButtonUp"`
	PhysicalWheel        uint64 `json:"physicalWheel"`
	InjectedWheel        uint64 `json:"injectedWheel"`
	Other                uint64 `json:"other"`
	SelfTestInjectedMove uint64 `json:"-"`
}

type SelfTestResult struct {
	SchemaVersion   string `json:"schemaVersion"`
	Passed          bool   `json:"passed"`
	Sent            uint32 `json:"sent"`
	Observed        uint64 `json:"observed"`
	InjectedMoves   uint64 `json:"injectedMoves"`
	PhysicalMoves   uint64 `json:"physicalMoves"`
	PointerRestored bool   `json:"pointerRestored"`
	FailureCode     string `json:"failureCode,omitempty"`
}

type StressSelfTestReport struct {
	SchemaVersion        string          `json:"schemaVersion"`
	RequestedDurationMs  int64           `json:"requestedDurationMillis"`
	ElapsedMillis        int64           `json:"elapsedMillis"`
	Attempts             uint64          `json:"attempts"`
	PassedAttempts       uint64          `json:"passedAttempts"`
	InterferenceAttempts uint64          `json:"interferenceAttempts"`
	Completed            bool            `json:"completed"`
	FailureAttempt       uint64          `json:"failureAttempt,omitempty"`
	FailureCode          string          `json:"failureCode,omitempty"`
	LastResult           *SelfTestResult `json:"lastResult,omitempty"`
}

type ProbeReport struct {
	SchemaVersion       string           `json:"schemaVersion"`
	DeviceAlias         string           `json:"deviceAlias"`
	CollectedAt         time.Time        `json:"collectedAt"`
	BuildRevision       string           `json:"buildRevision"`
	BuildModified       bool             `json:"buildModified"`
	RequestedDurationMs int64            `json:"requestedDurationMillis"`
	Completed           bool             `json:"completed"`
	FailureCode         string           `json:"failureCode,omitempty"`
	Diagnostics         *Diagnostics     `json:"diagnostics,omitempty"`
	Capture             *CaptureSnapshot `json:"capture,omitempty"`
	SelfTest            *SelfTestResult  `json:"selfTest,omitempty"`
}

type GuidedProbeReport struct {
	SchemaVersion       string             `json:"schemaVersion"`
	DeviceAlias         string             `json:"deviceAlias"`
	CollectedAt         time.Time          `json:"collectedAt"`
	BuildRevision       string             `json:"buildRevision"`
	BuildModified       bool               `json:"buildModified"`
	StageDurationMillis int64              `json:"stageDurationMillis"`
	Completed           bool               `json:"completed"`
	FailureCode         string             `json:"failureCode,omitempty"`
	Diagnostics         *Diagnostics       `json:"diagnostics,omitempty"`
	Stages              []GuidedProbeStage `json:"stages"`
	SelfTest            *SelfTestResult    `json:"selfTest,omitempty"`
}

type GuidedProbeStage struct {
	Name         string          `json:"name"`
	Passed       bool            `json:"passed"`
	FailureCodes []string        `json:"failureCodes"`
	Capture      CaptureSnapshot `json:"capture"`
}
