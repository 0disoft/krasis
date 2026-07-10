package lab

import (
	"context"
	"errors"
	"time"
)

const (
	DiagnoseSchemaVersion = "krasis.lab.diagnose/v1"
	CaptureSchemaVersion  = "krasis.lab.capture/v1"
	SelfTestSchemaVersion = "krasis.lab.selftest-inject/v1"
	ProbeSchemaVersion    = "krasis.lab.probe/v1"
)

var ErrUnsupported = errors.New("operation is not supported on this platform")

type Platform interface {
	Diagnose(context.Context) (Diagnostics, error)
	Capture(context.Context, CaptureOptions, func(CaptureSnapshot)) error
	SelfTestInject(context.Context) (SelfTestResult, error)
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
	PointerRestored bool   `json:"pointerRestored"`
	FailureCode     string `json:"failureCode,omitempty"`
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
