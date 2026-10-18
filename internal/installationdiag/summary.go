// Package installationdiag defines the bounded public installer receipt.
package installationdiag

import "time"

type Summary struct {
	Time               string `json:"time"`
	Action             string `json:"action"`
	Result             uint8  `json:"result"`
	Stage              string `json:"stage"`
	Phase              string `json:"phase,omitempty"`
	Operation          string `json:"operation,omitempty"`
	Win32Error         uint32 `json:"win32Error"`
	Win32Known         bool   `json:"win32Known"`
	Recovery           string `json:"recovery"`
	RecoveryStage      string `json:"recoveryStage,omitempty"`
	RecoveryPhase      string `json:"recoveryPhase,omitempty"`
	RecoveryOperation  string `json:"recoveryOperation,omitempty"`
	RecoveryWin32Error uint32 `json:"recoveryWin32Error"`
	RecoveryWin32Known bool   `json:"recoveryWin32Known"`
	CloseOperation     string `json:"closeOperation,omitempty"`
	CloseWin32Error    uint32 `json:"closeWin32Error"`
	CloseWin32Known    bool   `json:"closeWin32Known"`
	InProgress         bool   `json:"inProgress,omitempty"`
	ActiveStage        string `json:"activeStage,omitempty"`
	ActivePhase        string `json:"activePhase,omitempty"`
}

func (s Summary) Valid() bool {
	if _, err := time.Parse(time.RFC3339Nano, s.Time); err != nil {
		return false
	}
	if s.Action != "install" && s.Action != "remove" {
		return false
	}
	if s.Result != 0 && s.Result != 3 && s.Result != 4 && s.Result != 5 {
		return false
	}
	if s.InProgress {
		if s.Result != 5 || !stage(s.ActiveStage, false) || !phase(s.ActivePhase) || s.Recovery != "required" {
			return false
		}
	} else if s.ActiveStage != "" || s.ActivePhase != "" {
		return false
	}
	if !stage(s.Stage, false) || !stage(s.RecoveryStage, true) || !phase(s.Phase) || !phase(s.RecoveryPhase) || !operation(s.Operation) || !operation(s.RecoveryOperation) || !operation(s.CloseOperation) {
		return false
	}
	if !native(s.Win32Known, s.Win32Error) || !native(s.RecoveryWin32Known, s.RecoveryWin32Error) || !native(s.CloseWin32Known, s.CloseWin32Error) {
		return false
	}
	switch s.Recovery {
	case "not_attempted", "not_needed", "restored", "required", "journal_unknown", "committed":
		return true
	}
	return false
}

func native(known bool, code uint32) bool { return known && code != 0 || !known && code == 0 }
func stage(value string, optional bool) bool {
	if value == "" {
		return optional
	}
	switch value {
	case "validation", "payload", "backend", "lock", "load", "snapshot", "save", "stage", "stop", "replace", "start", "verify", "restore", "finish", "close", "complete":
		return true
	}
	return false
}
func operation(value string) bool {
	switch value {
	case "service_repair_retired", "service_repair_start", "service_repair_stop":
		return true
	case "", "file_open", "file_attributes", "file_security", "directory_create", "file_write", "file_sync", "file_close", "file_move", "directory_remove", "file_delete", "file_read", "known_folder", "scm_open", "scm_close", "service_open", "service_config", "service_status", "service_security", "service_ready", "service_start", "service_stop", "service_create", "service_delete":
		return true
	}
	return false
}

func phase(value string) bool {
	switch value {
	case "", "stage", "stop", "replace", "start", "commit", "rollback_stop", "rollback_restore", "rollback_start", "rollback_commit":
		return true
	}
	return false
}
