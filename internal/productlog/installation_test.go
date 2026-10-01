package productlog

import (
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/installationdiag"
)

func TestInstallationReceiptSurvivesExportAndPeerSanitization(t *testing.T) {
	base := t.TempDir()
	s := installationdiag.Summary{Time: "2026-10-02T00:00:00Z", Action: "install", Result: 5, Stage: "replace", Phase: "replace", Operation: "file_move", Win32Error: 32, Win32Known: true, Recovery: "required", RecoveryStage: "restore", RecoveryPhase: "rollback_restore", RecoveryOperation: "file_delete", RecoveryWin32Error: 5, RecoveryWin32Known: true}
	if err := SaveInstallation(base, s); err != nil {
		t.Fatal(err)
	}
	path, err := Export(base)
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data, err = SanitizeSupport(data)
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Installation installationdiag.Summary `json:"installation"`
	}
	if json.Unmarshal(data, &doc) != nil || doc.Installation != s {
		t.Fatal("installation failure lost in peer transfer")
	}
	s.Operation = "PRIVATE-PATH"
	if SaveInstallation(base, s) == nil {
		t.Fatal("arbitrary diagnostic string accepted")
	}
	bad, _ := json.Marshal(s)
	input := []byte(`{"schema":"krasis.support/v1","created":"2026-10-02T00:00:00Z","logs":[],"installation":` + string(bad) + `}`)
	clean, err := SanitizeSupport(input)
	if err != nil || strings.Contains(string(clean), "PRIVATE") || !strings.Contains(string(clean), "installationIncomplete") {
		t.Fatal("private receipt was exported")
	}
}

func TestIncompleteInstallationReplacesStaleReceiptAndSurvivesTransfer(t *testing.T) {
	base := t.TempDir()
	s := installationdiag.Summary{Time: "2026-10-02T00:00:00Z", Action: "install", Result: 5, Stage: "save", Recovery: "journal_unknown"}
	if err := SaveInstallation(base, s); err != nil {
		t.Fatal(err)
	}
	s.Time = "2026-10-02T01:00:00Z"
	s.Stage, s.Phase, s.Recovery = "replace", "replace", "required"
	s.InProgress, s.ActiveStage, s.ActivePhase = true, "replace", "replace"
	if err := SaveInstallation(base, s); err != nil {
		t.Fatal(err)
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
	var doc struct {
		Installation installationdiag.Summary `json:"installation"`
	}
	if err != nil || json.Unmarshal(clean, &doc) != nil || doc.Installation != s {
		t.Fatal("current incomplete installation lost in export")
	}
	s.Result = 0
	if s.Valid() {
		t.Fatal("pending checkpoint accepted as confirmed success")
	}
	s.Result = 5
	s.ActiveStage = "PRIVATE-PATH"
	if s.Valid() {
		t.Fatal("private checkpoint accepted")
	}
}
