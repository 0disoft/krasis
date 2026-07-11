package guardreport

import (
	"time"

	"github.com/0disoft/krasis/internal/buildinfo"
	"github.com/0disoft/krasis/internal/crashguard"
)

const SchemaVersion = "krasis.lab.crash-guard/v1"

type Report struct {
	SchemaVersion string    `json:"schemaVersion"`
	DeviceAlias   string    `json:"deviceAlias"`
	CollectedAt   time.Time `json:"collectedAt"`
	BuildRevision string    `json:"buildRevision"`
	BuildModified bool      `json:"buildModified"`
	Reason        string    `json:"reason"`
	Attempted     int       `json:"attempted"`
	Released      int       `json:"released"`
	Remaining     int       `json:"remaining"`
	Completed     bool      `json:"completed"`
	Passed        bool      `json:"passed"`
	FailureCode   string    `json:"failureCode,omitempty"`
}

func New(device string, build buildinfo.Info, result crashguard.Result, serveErr error) Report {
	report := Report{
		SchemaVersion: SchemaVersion, DeviceAlias: device, CollectedAt: time.Now().UTC(),
		BuildRevision: build.Revision, BuildModified: build.Modified, Reason: string(result.Reason),
		Attempted: result.Attempted, Released: result.Released, Remaining: result.Remaining, Completed: true,
	}
	if serveErr == nil && result.Remaining == 0 {
		report.Passed = true
		return report
	}
	if result.Reason == crashguard.ExitProtocolError {
		report.FailureCode = "protocol_error"
	} else {
		report.FailureCode = "release_failed"
	}
	return report
}
