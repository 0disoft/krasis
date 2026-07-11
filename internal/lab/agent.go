package lab

import "time"

const AgentSessionSchemaVersion = "krasis.lab.agent-session/v3"

type AgentSessionReport struct {
	SchemaVersion    string             `json:"schemaVersion"`
	DeviceAlias      string             `json:"deviceAlias"`
	Role             string             `json:"role"`
	Edge             string             `json:"edge"`
	CollectedAt      time.Time          `json:"collectedAt"`
	BuildRevision    string             `json:"buildRevision"`
	BuildModified    bool               `json:"buildModified"`
	RequestedMillis  int64              `json:"requestedDurationMillis"`
	Completed        bool               `json:"completed"`
	Passed           bool               `json:"passed"`
	StopReason       string             `json:"stopReason"`
	FailureCode      string             `json:"failureCode,omitempty"`
	CrashGuard       CrashGuardEvidence `json:"crashGuard"`
	Supervisor       SupervisorEvidence `json:"supervisor"`
	DestinationInput InputEvidence      `json:"destinationInput"`
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
