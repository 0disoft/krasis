package logindiag

import "time"

// Runtime records service decisions, not live status or permission. It contains
// no session/user identifier, endpoint, input value or raw native error text.
type Runtime struct {
	StartedAt   string          `json:"startedAt"`
	Time        string          `json:"time"`
	Service     string          `json:"service"`
	Console     string          `json:"console"`
	Grant       string          `json:"grant"`
	LastWorker  string          `json:"lastWorker"`
	LastFailure *RuntimeFailure `json:"lastFailure,omitempty"`
}
type RuntimeFailure struct {
	Time       string `json:"time"`
	Stage      string `json:"stage"`
	Win32Error uint32 `json:"win32Error"`
	Win32Known bool   `json:"win32Known"`
}

func (v Runtime) Valid() bool {
	started, err := time.Parse(time.RFC3339Nano, v.StartedAt)
	if err != nil {
		return false
	}
	stamp, err := time.Parse(time.RFC3339Nano, v.Time)
	if err != nil || stamp.Before(started) {
		return false
	}
	if v.Service != "STARTING" && v.Service != "READY" && v.Service != "STOPPED" && v.Service != "FAILED" {
		return false
	}
	if v.Console != "UNKNOWN" && v.Console != "BEFORE_LOGIN" && v.Console != "USER_SESSION" && v.Console != "UNAVAILABLE" {
		return false
	}
	if v.Grant != "UNKNOWN" && v.Grant != "PRESENT" && v.Grant != "MISSING" && v.Grant != "INVALID" {
		return false
	}
	if v.LastWorker != "NONE" && v.LastWorker != "STARTING" && v.LastWorker != "STARTED" && v.LastWorker != "FAILED" {
		return false
	}
	if f := v.LastFailure; f != nil {
		if _, err := time.Parse(time.RFC3339Nano, f.Time); err != nil {
			return false
		}
		if f.Stage != "console" && f.Stage != "grant" && f.Stage != "start" && f.Stage != "service" {
			return false
		}
		if !(f.Win32Known && f.Win32Error != 0 || !f.Win32Known && f.Win32Error == 0) {
			return false
		}
	}
	return true
}
