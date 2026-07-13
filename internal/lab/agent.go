package lab

import "time"

const AgentSessionSchemaVersion = "krasis.lab.agent-session/v6"

type AgentSessionReport struct {
	SchemaVersion     string             `json:"schemaVersion"`
	DeviceAlias       string             `json:"deviceAlias"`
	Role              string             `json:"role"`
	Edge              string             `json:"edge"`
	CollectedAt       time.Time          `json:"collectedAt"`
	BuildRevision     string             `json:"buildRevision"`
	BuildModified     bool               `json:"buildModified"`
	RequestedMillis   int64              `json:"requestedDurationMillis"`
	HeartbeatMillis   int64              `json:"heartbeatIntervalMillis"`
	PeerTimeoutMillis int64              `json:"peerTimeoutMillis"`
	Completed         bool               `json:"completed"`
	Passed            bool               `json:"passed"`
	StopReason        string             `json:"stopReason"`
	FailureCode       string             `json:"failureCode,omitempty"`
	Lifecycle         LifecycleEvidence  `json:"lifecycle"`
	CrashGuard        CrashGuardEvidence `json:"crashGuard"`
	Supervisor        SupervisorEvidence `json:"supervisor"`
	Liveness          LivenessEvidence   `json:"liveness"`
	DestinationInput  InputEvidence      `json:"destinationInput"`
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
	FinalState      string `json:"finalState"`
	ActiveCount     uint64 `json:"activeCount"`
	AcquireFailures uint64 `json:"acquireFailures"`
	SessionFailures uint64 `json:"sessionFailures"`
	EmergencyStops  uint64 `json:"emergencyStops"`
}

type InputEvidence struct {
	Entries      uint64 `json:"entries"`
	KeyDowns     uint64 `json:"keyDowns"`
	KeyUps       uint64 `json:"keyUps"`
	ButtonDowns  uint64 `json:"buttonDowns"`
	ButtonUps    uint64 `json:"buttonUps"`
	PointerMoves uint64 `json:"pointerMoves"`
	Wheels       uint64 `json:"wheels"`
	ReturnArms   uint64 `json:"returnArms"`
}
