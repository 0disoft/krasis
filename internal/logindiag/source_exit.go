package logindiag

import "time"

// SourceExit records one source-only lifetime. It is never native retirement
// proof or input authority and contains no operation payload or input content.
type SourceExit struct {
	Time       string         `json:"time,omitempty"`
	Command    string         `json:"command"`
	Reader     string         `json:"reader"`
	Capture    string         `json:"capture"`
	Sender     string         `json:"sender"`
	Interrupt  string         `json:"interrupt"`
	Retirement string         `json:"retirement"`
	Report     string         `json:"report"`
	Failure    *SourceFailure `json:"failure,omitempty"`
}

// Added by the service after consuming the final worker report and joining its
// owner. Report status and native retirement remain separate from this cause.
// No child-provided error text or operation payload is retained.
type SourceFailure struct {
	Operation  string `json:"operation,omitempty"`
	Win32Error uint32 `json:"win32Error"`
	Win32Known bool   `json:"win32Known"`
}

func (v SourceFailure) Valid() bool {
	return ValidRuntimeOperation(v.Operation) &&
		(v.Win32Known && v.Win32Error != 0 || !v.Win32Known && v.Win32Error == 0)
}

type SourceHistory struct {
	Total uint64       `json:"total"`
	Exits []SourceExit `json:"exits"`
}

const MaxSourceExits = 16

func (v SourceExit) ValidDetails() bool {
	if v.Failure != nil && !v.Failure.Valid() {
		return false
	}
	if v.Report != "recovery" && (v.Sender == "unjoined" || v.Interrupt == "unjoined" || v.Capture == "failed") {
		return false
	}
	return member(v.Command, "not_started", "ok", "inspection", "authority", "protocol", "cleanup", "cancelled", "failed") &&
		member(v.Reader, "not_started", "stopped", "cancelled", "eof", "truncated", "timeout", "closed", "invalid", "authority", "failed") &&
		member(v.Capture, "not_started", "joined", "failed") &&
		member(v.Sender, "not_started", "joined", "unjoined") &&
		member(v.Interrupt, "not_started", "joined", "unjoined") &&
		member(v.Retirement, "not_sent", "clean_sent", "failed_sent", "delivery_failed") &&
		member(v.Report, "ok", "failed", "recovery")
}

func (v SourceHistory) Valid(started, now time.Time) bool {
	if len(v.Exits) == 0 || len(v.Exits) > MaxSourceExits || v.Total < uint64(len(v.Exits)) {
		return false
	}
	previous := started
	for _, exit := range v.Exits {
		stamp, err := time.Parse(time.RFC3339Nano, exit.Time)
		if err != nil || stamp.Before(previous) || stamp.After(now) || !exit.ValidDetails() {
			return false
		}
		previous = stamp
	}
	return true
}

func member(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
