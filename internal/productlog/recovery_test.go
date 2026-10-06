package productlog

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"

	"github.com/0disoft/krasis/internal/peercredentials"
	"github.com/0disoft/krasis/internal/productidentity"
)

func TestRecoveryRetainsLastFailureAfterHealthyChecks(t *testing.T) {
	base := t.TempDir()
	failure := RecoverySummary{Time: "2026-10-07T00:00:00Z", Code: "agent_faulted", Generation: 198, AgentState: "FAULTED"}
	if err := SaveRecovery(base, failure); err != nil {
		t.Fatal(err)
	}
	for _, code := range []string{"launch_requested", "agent_present", "recovery_disabled"} {
		current := RecoverySummary{Time: "2026-10-07T00:01:00Z", Code: code, Generation: 199, AgentState: "CONNECTED"}
		if err := SaveRecovery(base, current); err != nil {
			t.Fatal(err)
		}
		got, err := RecoveryReceipt(base)
		if err != nil || got == nil || got.Code != code || got.LastFailure == nil || *got.LastFailure != *failure.failure() {
			t.Fatal(got, err)
		}
	}
	path, err := Export(base)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	clean, err := SanitizeSupport(data)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Recovery *RecoverySummary `json:"recovery"`
	}
	if err = json.Unmarshal(clean, &doc); err != nil || doc.Recovery == nil || doc.Recovery.LastFailure == nil || *doc.Recovery.LastFailure != *failure.failure() {
		t.Fatal(doc, err)
	}
	failure.Time, failure.Code = "2026-10-07T00:02:00Z", "launch_failed"
	if err = SaveRecovery(base, failure); err != nil {
		t.Fatal(err)
	}
	got, err := RecoveryReceipt(base)
	if err != nil || got == nil || got.LastFailure == nil || *got.LastFailure != *failure.failure() {
		t.Fatal(got, err)
	}
}

func TestRemoteRecoveryLastFailureRejectsPrivateAndInvalidMetadata(t *testing.T) {
	for _, code := range []string{"launch_failed", "agent_present", "SECRET"} {
		raw := []byte(`{"schema":"krasis.support/v1","created":"2026-10-07T00:00:00Z","logs":[],"recovery":{"time":"2026-10-07T00:01:00Z","code":"agent_present","lastFailure":{"time":"2026-10-07T00:00:00Z","code":"` + code + `","path":"SECRET"}}}`)
		clean, err := SanitizeSupport(raw)
		if err != nil || bytes.Contains(clean, []byte("SECRET")) {
			t.Fatal(err)
		}
		var doc struct {
			Recovery   *RecoverySummary `json:"recovery"`
			Incomplete bool             `json:"recoveryIncomplete"`
		}
		if err = json.Unmarshal(clean, &doc); err != nil {
			t.Fatal(err)
		}
		if code == "launch_failed" {
			if doc.Recovery == nil || doc.Recovery.LastFailure == nil || doc.Incomplete {
				t.Fatal(string(clean))
			}
		} else if doc.Recovery != nil || !doc.Incomplete {
			t.Fatal("invalid failure retained", string(clean))
		}
	}
}

func TestLegacyRecoveryFailureSurvivesFirstHealthyCheck(t *testing.T) {
	base := t.TempDir()
	dir, err := productidentity.PrepareDiagnostics(base)
	if err != nil {
		t.Fatal(err)
	}
	defer dir.Close()
	f, err := dir.CreateExclusive("recovery-latest.json", 0600)
	if err != nil {
		t.Fatal(err)
	}
	if err = peercredentials.ProtectSecurityFileHandle(f); err != nil {
		f.Close()
		t.Fatal(err)
	}
	legacy := RecoverySummary{Time: "2026-10-07T00:00:00Z", Code: "launch_failed", Generation: 198}
	if err = json.NewEncoder(f).Encode(legacy); err != nil {
		f.Close()
		t.Fatal(err)
	}
	if err = f.Close(); err != nil {
		t.Fatal(err)
	}
	if err = SaveRecovery(base, RecoverySummary{Time: "2026-10-07T00:01:00Z", Code: "agent_present"}); err != nil {
		t.Fatal(err)
	}
	got, err := RecoveryReceipt(base)
	if err != nil || got == nil || got.LastFailure == nil || *got.LastFailure != *legacy.failure() {
		t.Fatal(got, err)
	}
}
