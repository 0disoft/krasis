package productlog

import (
	"encoding/json"
	"github.com/0disoft/krasis/internal/logindiag"
	"os"
	"strings"
	"testing"
)

func TestApprovalReceiptSurvivesPeerTransferAndRejectsPrivateDetail(t *testing.T) {
	base := t.TempDir()
	s := logindiag.Summary{Time: "2026-10-11T00:00:00Z", Action: "prepare", Result: 3, Stage: "local_credential", Win32Error: 5, Win32Known: true}
	if err := SaveLoginConfiguration(base, s); err != nil {
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
	if err != nil {
		t.Fatal(err)
	}
	var doc struct {
		Login logindiag.Summary `json:"loginConfiguration"`
	}
	if json.Unmarshal(clean, &doc) != nil || doc.Login != s {
		t.Fatal("approval receipt lost in transfer", string(clean))
	}
	s.Stage = "SECRET/profile/path"
	if SaveLoginConfiguration(base, s) == nil {
		t.Fatal("invalid stage persisted")
	}
	bad, _ := json.Marshal(s)
	input := []byte(`{"schema":"krasis.support/v1","created":"2026-10-11T00:00:00Z","logs":[],"loginConfiguration":` + string(bad) + `}`)
	clean, err = SanitizeSupport(input)
	if err != nil || strings.Contains(string(clean), "SECRET") || !strings.Contains(string(clean), "loginConfigurationIncomplete") {
		t.Fatal("invalid receipt leaked")
	}
}
