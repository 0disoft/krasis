package productlog

import (
	"bytes"
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type RecoverySummary struct {
	Time              string           `json:"time"`
	Code              string           `json:"code"`
	Generation        int              `json:"generation,omitempty"`
	AgentState        string           `json:"agentState,omitempty"`
	LastFailure       *RecoveryFailure `json:"lastFailure,omitempty"`
	HistoryIncomplete bool             `json:"historyIncomplete,omitempty"`
}

// A separate non-recursive shape keeps received diagnostic metadata bounded.
type RecoveryFailure struct {
	Time       string `json:"time"`
	Code       string `json:"code"`
	Generation int    `json:"generation,omitempty"`
	AgentState string `json:"agentState,omitempty"`
}

var recoveryGate sync.Mutex
var errInvalidRecoveryReceipt = errors.New("invalid recovery receipt")
var errRecoverySavePending = errors.New("recovery receipt save pending")

const recoveryPendingName = "recovery-pending.json"

func (v RecoverySummary) failed() bool {
	switch v.Code {
	case "settings_unavailable", "agent_unavailable", "agent_faulted", "agent_unverified", "launch_record_invalid", "launch_failed", "launch_record_missing":
		return true
	}
	return false
}

func (v RecoverySummary) failure() *RecoveryFailure {
	return &RecoveryFailure{Time: v.Time, Code: v.Code, Generation: v.Generation, AgentState: v.AgentState}
}

func (v RecoverySummary) valid() bool {
	if f := v.LastFailure; f != nil {
		event := RecoverySummary{Time: f.Time, Code: f.Code, Generation: f.Generation, AgentState: f.AgentState}
		if !event.valid() || !event.failed() {
			return false
		}
	}
	if _, err := time.Parse(time.RFC3339Nano, v.Time); err != nil || v.Generation < 0 || v.Generation > 1000000 {
		return false
	}
	switch v.AgentState {
	case "", "LOCAL", "CONNECTED", "CONNECTING", "RECONNECTING", "DISCONNECTING", "FAULTED", "UNKNOWN":
	default:
		return false
	}
	switch v.Code {
	case "settings_unavailable", "recovery_disabled", "agent_unavailable", "agent_present", "agent_faulted", "agent_unverified", "launch_record_invalid", "launch_requested", "launch_failed", "launch_record_missing":
		return true
	}
	return false
}

func SaveRecovery(base string, value RecoverySummary) error {
	if !value.valid() {
		return errors.New("invalid recovery receipt")
	}
	recoveryGate.Lock()
	defer recoveryGate.Unlock()
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return err
	}
	defer dir.Close()
	if err = resumeRecoverySave(dir); err != nil {
		return err
	}
	previous, err := readRecoveryReceipt(dir)
	corrupt := errors.Is(err, errInvalidRecoveryReceipt)
	if err != nil && !corrupt {
		return err
	}
	value.HistoryIncomplete = corrupt || (previous != nil && previous.HistoryIncomplete)
	if previous == nil && !corrupt {
		value.HistoryIncomplete, err = hasCorruptRecoveryArchive(dir)
		if err != nil {
			return err
		}
	}
	value.LastFailure = nil
	if value.failed() {
		value.LastFailure = value.failure()
	} else if previous != nil {
		value.LastFailure = previous.LastFailure
		if value.LastFailure == nil && previous.failed() {
			value.LastFailure = previous.failure()
		}
	}
	f, name, err := dir.CreateTemporary(".recovery-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(name)
	defer f.Close()
	if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
		return err
	}
	if err = json.NewEncoder(f).Encode(value); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if corrupt {
		if err = dir.PromoteNoReplace(name, recoveryPendingName); err != nil {
			return err
		}
		if err = archiveCorruptRecovery(dir); err != nil {
			return err
		}
		return dir.Replace(recoveryPendingName, "recovery-latest.json")
	}
	return dir.Replace(name, "recovery-latest.json")
}

func RecoveryReceipt(base string) (*RecoverySummary, error) {
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return nil, err
	}
	if _, err = os.Lstat(paths.Diagnostics); errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if _, err = peercredentials.ValidateSecurityDirectory(paths.Diagnostics); err != nil {
		return nil, err
	}
	dir, err := securefs.OpenReadDirectory(filepath.Clean(paths.Diagnostics))
	if err != nil {
		return nil, err
	}
	defer dir.Close()
	return readRecoveryReceipt(dir)
}

func readRecoveryReceipt(dir *securefs.Directory) (*RecoverySummary, error) {
	if pending, err := readRecoveryReceiptNamed(dir, recoveryPendingName); err != nil {
		return nil, err
	} else if pending != nil {
		return nil, errRecoverySavePending
	}
	return readRecoveryReceiptNamed(dir, "recovery-latest.json")
}

func readRecoveryReceiptNamed(dir *securefs.Directory, name string) (*RecoverySummary, error) {
	f, err := dir.Open(name)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer f.Close()
	if err = peercredentials.ValidateSecurityFileHandle(f); err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(f, 1025))
	if err != nil {
		return nil, err
	}
	if len(data) > 1024 {
		return nil, errInvalidRecoveryReceipt
	}
	d := json.NewDecoder(bytes.NewReader(data))
	d.DisallowUnknownFields()
	var value RecoverySummary
	if d.Decode(&value) != nil || !value.valid() || d.Decode(new(any)) != io.EOF {
		return nil, errInvalidRecoveryReceipt
	}
	return &value, nil
}

func init() {
	for _, code := range []string{"peer_update_apply_timeout", "release_check_failed", "ui_replacement_bundle_failed", "ui_replacement_stage_failed", "ui_replacement_waiting_for_idle", "ui_replacement_launch_failed", "peer_stage_cleanup_failed", "peer_stage_cleanup_completed"} {
		allowed[code] = true
	}
}
