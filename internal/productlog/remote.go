package productlog

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/installationdiag"
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
	"os"
	"path/filepath"
	"time"
)

// Four bounded log files plus the support document envelope.
const MaxSupportBytes = 5 << 20

// SanitizeSupport reprojects an untrusted support document, never raw logs.
func SanitizeSupport(data []byte) ([]byte, error) {
	if len(data) > MaxSupportBytes {
		return nil, errors.New("support size exceeded")
	}
	var doc struct {
		Schema                       string                    `json:"schema"`
		Created                      string                    `json:"created"`
		Scope                        string                    `json:"scope"`
		Logs                         boundedSupportLogs        `json:"logs"`
		Installation                 *installationdiag.Summary `json:"installation,omitempty"`
		InstallationIncomplete       bool                      `json:"installationIncomplete,omitempty"`
		Recovery                     *RecoverySummary          `json:"recovery,omitempty"`
		RecoveryIncomplete           bool                      `json:"recoveryIncomplete,omitempty"`
		Cleanup                      *CleanupSummary           `json:"cleanup,omitempty"`
		CleanupIncomplete            bool                      `json:"cleanupIncomplete,omitempty"`
		LoginConfiguration           *logindiag.Summary        `json:"loginConfiguration,omitempty"`
		LoginConfigurationIncomplete bool                      `json:"loginConfigurationIncomplete,omitempty"`
		LoginRuntime                 *boundedLoginRuntime      `json:"loginRuntime,omitempty"`
		LoginRuntimeIncomplete       bool                      `json:"loginRuntimeIncomplete,omitempty"`
		InputHealth                  *InputHealthSummary       `json:"inputHealth,omitempty"`
		InputHealthIncomplete        bool                      `json:"inputHealthIncomplete,omitempty"`
	}
	if json.Unmarshal(data, &doc) != nil || doc.Schema != "krasis.support/v1" || len(doc.Logs) > 4 {
		return nil, errors.New("invalid support document")
	}
	created, err := time.Parse(time.RFC3339, doc.Created)
	if err != nil {
		return nil, err
	}
	doc.Created = created.UTC().Format(time.RFC3339)
	doc.Scope = "Event history only; received from a paired device; not a live status."
	if doc.InputHealth != nil && !doc.InputHealth.valid() {
		doc.InputHealth = nil
		doc.InputHealthIncomplete = true
	}
	if doc.LoginRuntime != nil && !doc.LoginRuntime.Valid() {
		doc.LoginRuntime = nil
		doc.LoginRuntimeIncomplete = true
	}
	if doc.LoginConfiguration != nil && !doc.LoginConfiguration.Valid() {
		doc.LoginConfiguration = nil
		doc.LoginConfigurationIncomplete = true
	}
	if doc.Installation != nil && !doc.Installation.Valid() {
		doc.Installation = nil
		doc.InstallationIncomplete = true
	}
	if doc.Recovery != nil && !doc.Recovery.valid() {
		doc.Recovery = nil
		doc.RecoveryIncomplete = true
	}
	if doc.Cleanup != nil && !doc.Cleanup.valid() {
		doc.Cleanup = nil
		doc.CleanupIncomplete = true
	}
	seen := map[string]bool{}
	for i, l := range doc.Logs {
		if seen[l.Source] || (l.Source != "agent.jsonl" && l.Source != "agent.previous.jsonl" && l.Source != "control.jsonl" && l.Source != "control.previous.jsonl") {
			return nil, errors.New("invalid support source")
		}
		seen[l.Source] = true
		var lines bytes.Buffer
		for _, event := range l.Events {
			if err := json.NewEncoder(&lines).Encode(event); err != nil {
				return nil, err
			}
		}
		clean := sanitizeLog(l.Source, &lines)
		clean.Incomplete = clean.Incomplete || l.Incomplete
		doc.Logs[i] = clean
	}
	return json.Marshal(doc)
}

func SaveRemoteSupport(base, peer string, data []byte) (string, error) {
	return saveRemoteSupport(base, peer, "", data)
}

func SaveRemoteSupportForRequest(base, peer, id string, data []byte) (string, error) {
	if !validRequestID(id) {
		return "", errors.New("invalid diagnostic request")
	}
	return saveRemoteSupport(base, peer, id, data)
}

func saveRemoteSupport(base, peer, id string, data []byte) (string, error) {
	decoded, err := hex.DecodeString(peer)
	if err != nil || len(decoded) != 32 {
		return "", errors.New("invalid peer")
	}
	clean, err := SanitizeSupport(data)
	if err != nil {
		return "", err
	}
	if id != "" {
		requestSnapshotGate.Lock()
		defer requestSnapshotGate.Unlock()
	}
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		return "", err
	}
	defer dir.Close()
	name := "support-peer-" + peer + ".json"
	if id != "" {
		name = "support-request-" + id + "-peer-" + peer + ".json"
		if err = reserveRequestSnapshot(dir, id, name, int64(len(clean))); err != nil {
			return "", err
		}
	}
	f, temp, err := dir.CreateTemporary(".peer-support-", 0600)
	if err != nil {
		return "", err
	}
	defer dir.Remove(temp)
	defer f.Close()
	if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
		return "", err
	}
	if _, err = f.Write(clean); err != nil {
		return "", err
	}
	if err = f.Sync(); err != nil {
		return "", err
	}
	if err = f.Close(); err != nil {
		return "", err
	}
	if id != "" {
		err = dir.PromoteNoReplace(temp, name)
		if errors.Is(err, os.ErrExist) {
			var old []byte
			old, err = readSupportSnapshot(dir, name)
			if err == nil && !bytes.Equal(old, clean) {
				err = errors.New("conflicting diagnostic snapshot")
			}
		}
	} else {
		err = dir.Replace(temp, name)
	}
	if err != nil {
		return "", err
	}
	paths, err := productidentity.ResolveWindowsPaths(base)
	if err != nil {
		return "", err
	}
	return filepath.Join(paths.Diagnostics, name), nil
}
