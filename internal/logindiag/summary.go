// Package logindiag owns bounded approval diagnostics, never credentials or paths.
package logindiag

import (
	"errors"
	"syscall"
	"time"
)

type Summary struct {
	Time       string `json:"time"`
	Action     string `json:"action"`
	Result     int    `json:"result"`
	Stage      string `json:"stage"`
	Win32Error uint32 `json:"win32Error"`
	Win32Known bool   `json:"win32Known"`
}

type failure struct {
	stage string
	cause error
}

func (e *failure) Error() string { return "login configuration failed at " + e.stage }
func (e *failure) Unwrap() error { return e.cause }

// At keeps the earliest specific failing step when outer boundaries wrap it.
func At(stage string, err error) error {
	if err == nil {
		return nil
	}
	var previous *failure
	if errors.As(err, &previous) {
		return err
	}
	if !validStage(stage) {
		stage = "configuration"
	}
	return &failure{stage, err}
}

func New(action string, result int, err error) Summary {
	s := Summary{Time: time.Now().UTC().Format(time.RFC3339Nano), Action: action, Result: result, Stage: "complete"}
	if result == 2 {
		s.Stage = "arguments"
	} else if err != nil || result != 0 {
		s.Stage = "configuration"
		var detail *failure
		if errors.As(err, &detail) {
			s.Stage = detail.stage
		}
		var native syscall.Errno
		if errors.As(err, &native) && native != 0 {
			s.Win32Error = uint32(native)
			s.Win32Known = true
		}
	}
	return s
}

func (s Summary) Valid() bool {
	_, err := time.Parse(time.RFC3339Nano, s.Time)
	return err == nil && (s.Action == "prepare" || s.Action == "disable") &&
		(s.Result == 0 || s.Result == 2 || s.Result == 3 || s.Result == 5) && validStage(s.Stage) &&
		(s.Win32Known && s.Win32Error != 0 || !s.Win32Known && s.Win32Error == 0) &&
		((s.Result == 0) == (s.Stage == "complete")) && (s.Result != 2 || s.Stage == "arguments")
}

func validStage(s string) bool {
	switch s {
	case "complete", "arguments", "configuration", "owner_token", "owner_elevation", "owner_profile", "current_profile", "profile_mismatch", "startup_read", "resume_policy", "startup_policy", "peer_read", "local_credential", "local_key", "grant_id", "grant_validate", "prepare", "installation_read", "install_files", "install_lock", "transaction_read", "transaction_present", "policy_recheck", "grant_encode", "pending_write", "commit", "grant_verify", "committed_policy", "close", "remove":
		return true
	}
	return false
}
