package productlog

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"path/filepath"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/installationdiag"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/securefs"
)

const ExportName = "support-latest.json"

type exportEvent struct {
	Time          string            `json:"time"`
	Code          string            `json:"code"`
	Build         string            `json:"build,omitempty"`
	BuildDirty    bool              `json:"buildDirty,omitempty"`
	Run           string            `json:"run,omitempty"`
	Connection    string            `json:"connection,omitempty"`
	Sequence      uint64            `json:"sequence,omitempty"`
	ElapsedMicros int64             `json:"elapsedMicros,omitempty"`
	Boundary      *BoundarySummary  `json:"boundary,omitempty"`
	Injection     *InjectionFailure `json:"injection,omitempty"`
	Retry         *RetrySummary     `json:"retry,omitempty"`
	Runtime       *RuntimeSummary   `json:"runtime,omitempty"`
	Handoff       *handoff.Summary  `json:"handoff,omitempty"`
}

type exportLog struct {
	Source     string        `json:"source"`
	Incomplete bool          `json:"incomplete"`
	Events     []exportEvent `json:"events"`
}

// Export saves a bounded, sanitized snapshot, replacing only the previous support
// snapshot. It does not start the agent, inspect credentials, or upload anything.
func Export(localAppData string) (string, error) {
	paths, err := productidentity.ResolveWindowsPaths(localAppData)
	if err != nil {
		return "", err
	}
	dir, err := productidentity.PrepareDiagnostics(localAppData)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	if err = exportTo(dir); err != nil {
		return "", err
	}
	return filepath.Join(paths.Diagnostics, ExportName), nil
}

func exportTo(dir *securefs.Directory) error {
	logs := make([]exportLog, 0, 4)
	for _, name := range []string{"agent.previous.jsonl", "agent.jsonl", "control.previous.jsonl", "control.jsonl"} {
		log := exportLog{Source: name, Incomplete: true, Events: []exportEvent{}}
		if f, err := dir.Open(name); err == nil {
			log = sanitizeLog(name, f)
			if f.Close() != nil {
				log.Incomplete = true
			}
		}
		logs = append(logs, log)
	}
	installation, receiptErr := readInstallationReceipt(dir)
	recovery, recoveryErr := readRecoveryReceipt(dir)
	cleanup, cleanupErr := readCleanupReceipt(dir)
	document := struct {
		Schema                 string                    `json:"schema"`
		Created                string                    `json:"created"`
		Scope                  string                    `json:"scope"`
		Logs                   []exportLog               `json:"logs"`
		Installation           *installationdiag.Summary `json:"installation,omitempty"`
		InstallationIncomplete bool                      `json:"installationIncomplete,omitempty"`
		Recovery               *RecoverySummary          `json:"recovery,omitempty"`
		RecoveryIncomplete     bool                      `json:"recoveryIncomplete,omitempty"`
		Cleanup                *CleanupSummary           `json:"cleanup,omitempty"`
		CleanupIncomplete      bool                      `json:"cleanupIncomplete,omitempty"`
	}{"krasis.support/v1", time.Now().UTC().Format(time.RFC3339), "Event history only; not a live agent status or complete crash report.", logs, installation, receiptErr != nil, recovery, recoveryErr != nil, cleanup, cleanupErr != nil}
	f, temp, err := dir.CreateTemporary(".support-", 0600)
	if err != nil {
		return err
	}
	defer dir.Remove(temp)
	defer f.Close()
	if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
		return err
	}
	if err = json.NewEncoder(f).Encode(document); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return dir.Replace(temp, ExportName)
}

func sanitizeLog(name string, r io.Reader) exportLog {
	result := exportLog{Source: name, Events: []exportEvent{}}
	data, err := io.ReadAll(io.LimitReader(r, maxBytes+1))
	if err != nil || len(data) > maxBytes {
		result.Incomplete = true
		return result
	}
	// Concurrent logging may leave an unfinished last record. Never export it.
	if len(data) > 0 && data[len(data)-1] != '\n' {
		result.Incomplete = true
		if last := bytes.LastIndexByte(data, '\n'); last >= 0 {
			data = data[:last+1]
		} else {
			data = nil
		}
	}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 1024), 4096)
	for scanner.Scan() {
		var input record
		if json.Unmarshal(scanner.Bytes(), &input) != nil || !allowed[input.Code] {
			result.Incomplete = true
			continue
		}
		timestamp, err := time.Parse(time.RFC3339Nano, input.Time)
		if err != nil {
			result.Incomplete = true
			continue
		}
		// Reproject only validated diagnostic identity. Never export arbitrary
		// build strings, process IDs, TLS material, endpoint addresses or raw lines.
		event := exportEvent{Time: timestamp.UTC().Format(time.RFC3339Nano), Code: input.Code}
		if validHex(input.Build, 40) || validHex(input.Build, 64) || input.Build == "development" {
			event.Build = input.Build
			event.BuildDirty = input.BuildDirty
		}
		if validHex(input.Run, 32) && input.Sequence > 0 && input.ElapsedMicros >= 0 {
			event.Run = input.Run
			event.Sequence = input.Sequence
			event.ElapsedMicros = input.ElapsedMicros
		}
		if input.Code != "handoff_profile" && validHex(input.Connection, 64) {
			event.Connection = input.Connection
		}
		if input.Code == "injection_failure" {
			if input.Injection == nil || !input.Injection.valid() {
				result.Incomplete = true
				continue
			}
			event.Injection = input.Injection
		}
		if input.Code == "retry_policy" {
			if input.Retry == nil || !input.Retry.valid() {
				result.Incomplete = true
				continue
			}
			event.Retry = input.Retry
		}
		if input.Code == "runtime_summary" {
			if input.Runtime == nil || !input.Runtime.valid() {
				result.Incomplete = true
				continue
			}
			event.Runtime = input.Runtime
		}
		if input.Code == "handoff_profile" {
			if input.Handoff == nil || !input.Handoff.Valid() {
				result.Incomplete = true
				continue
			}
			event.Handoff = input.Handoff
		}
		if input.Code == "boundary_summary" {
			if input.Boundary == nil || !input.Boundary.valid() {
				result.Incomplete = true
				continue
			}
			event.Boundary = input.Boundary
		}
		result.Events = append(result.Events, event)
	}
	if scanner.Err() != nil {
		result.Incomplete = true
	}
	return result
}
