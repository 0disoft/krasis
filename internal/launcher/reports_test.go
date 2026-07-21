package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestBundleReportsIncludesExactObjectsWithoutPaths(t *testing.T) {
	directory := t.TempDir()
	agentPath := filepath.Join(directory, "agent.json")
	guardPath := filepath.Join(directory, "guard.json")
	if err := os.WriteFile(agentPath, []byte(`{"schemaVersion":"agent/v1","completed":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guardPath, []byte(`{"schemaVersion":"guard/v1","passed":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	payload, err := BundleReports(agentPath, guardPath)
	if err != nil {
		t.Fatal(err)
	}
	var bundle struct {
		SchemaVersion string         `json:"schemaVersion"`
		Agent         map[string]any `json:"agent"`
		Guard         map[string]any `json:"guard"`
	}
	if err := json.Unmarshal(payload, &bundle); err != nil {
		t.Fatal(err)
	}
	if bundle.SchemaVersion != ReportBundleSchemaVersion || bundle.Agent["schemaVersion"] != "agent/v1" || bundle.Guard["schemaVersion"] != "guard/v1" {
		t.Fatalf("bundle = %+v", bundle)
	}
	if string(payload) == "" || containsText(string(payload), directory) {
		t.Fatalf("bundle leaked a local path: %s", payload)
	}
}

func TestBundleReportsRejectsTrailingJSON(t *testing.T) {
	directory := t.TempDir()
	agentPath := filepath.Join(directory, "agent.json")
	guardPath := filepath.Join(directory, "guard.json")
	if err := os.WriteFile(agentPath, []byte(`{} {}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(guardPath, []byte(`{}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := BundleReports(agentPath, guardPath); err == nil {
		t.Fatal("BundleReports accepted multiple JSON values")
	}
}

func containsText(value, target string) bool {
	for index := 0; index+len(target) <= len(value); index++ {
		if value[index:index+len(target)] == target {
			return true
		}
	}
	return false
}
