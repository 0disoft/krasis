package logindiag

import (
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

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
	Time       string      `json:"time"`
	Stage      string      `json:"stage"`
	Reason     string      `json:"reason,omitempty"`
	Operation  string      `json:"operation,omitempty"`
	Win32Error uint32      `json:"win32Error"`
	Win32Known bool        `json:"win32Known"`
	WorkerExit *WorkerExit `json:"workerExit,omitempty"`
}

// Process status is separate from an operation's Win32 error. An observed exit
// (including code zero) does not acknowledge native cleanup or released inputs.
type WorkerExit struct {
	State string `json:"state"`
	Code  uint32 `json:"code"`
	Known bool   `json:"known"`
}

func (v WorkerExit) Valid() bool {
	return (v.State == "RUNNING" || v.State == "UNAVAILABLE") && !v.Known && v.Code == 0 ||
		v.State == "EXITED" && (v.Known || v.Code == 0)
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
	if v.LastWorker != "NONE" && v.LastWorker != "STARTING" && v.LastWorker != "STARTED" && v.LastWorker != "FAILED" && v.LastWorker != "STOPPED" {
		return false
	}
	if f := v.LastFailure; f != nil {
		if _, err := time.Parse(time.RFC3339Nano, f.Time); err != nil {
			return false
		}
		if f.Stage != "console" && f.Stage != "grant" && f.Stage != "start" && f.Stage != "service" && f.Stage != "worker" {
			return false
		}
		if !ValidRuntimeReason(f.Reason) || !ValidRuntimeOperation(f.Operation) {
			return false
		}
		if !(f.Win32Known && f.Win32Error != 0 || !f.Win32Known && f.Win32Error == 0) {
			return false
		}
		if f.WorkerExit != nil && !f.WorkerExit.Valid() {
			return false
		}
	}
	return true
}

func ValidRuntimeOperation(operation string) bool {
	if inputdesktop.ValidCaptureThreadFailureOperation(operation) {
		return true
	}
	switch operation {
	case "source-reader-read", "source-reader-eof", "source-reader-truncated", "source-reader-timeout", "source-reader-closed", "source-reader-invalid", "source-reader-authority", "worker-report-eof", "worker-report-truncated", "worker-report-invalid":
		return true
	case "", "low-level-hook", "raw-mouse", "wheel-capture", "thread-join", "hook-install", "hook-release", "raw-register", "raw-release", "raw-read", "raw-buffer", "raw-route", "message-loop", "pointer-snapshot", "cursor-present", "cursor-restore", "cursor-release", "worker-report-read", "worker-report-timeout", "raw-size", "raw-packet", "raw-desktop", "desktop-bind", "desktop-restore", "desktop-close", "desktop-thread-exit", "raw-window-create", "raw-window-destroy", "raw-ready", "desktop-current", "desktop-open", "desktop-name", "desktop-active", "source-capture-join", "source-sender-join", "source-command-loop", "source-retirement-write", "source-channel-close":
		return true
	}
	return false
}

func ValidRuntimeReason(reason string) bool {
	switch reason {
	case "", "unknown", "desktop", "policy", "input", "capture", "cleanup", "connection", "launch", "pipe", "bootstrap", "activation":
		return true
	}
	return false
}
