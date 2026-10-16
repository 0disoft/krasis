package productlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/securefs"
)

// Exercise the production writer and export filter together: testing only the
// worker observer misses allowlist omissions that silently erase its evidence.
func TestInputFailurePhasesSurvivePersistenceAndExport(t *testing.T) {
	path := t.TempDir()
	dir, err := securefs.OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	logger := newLogger(dir, closer{}, "agent")
	defer logger.Close()
	stages := []string{"scope_admission", "apply", "cursor_readback", "scope_completion", "ack_mismatch", "ack_timeout", "ack_eof", "ack_closed", "ack_unclassified"}
	for _, stage := range stages {
		logger.Event("installed_input_failure_" + stage)
	}
	logger.Event("installed_input_failure_PRIVATE")
	logger.Close()
	data, err := os.ReadFile(filepath.Join(path, "agent.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE") {
		t.Fatal("dynamic failure text entered the persistent log")
	}
	result := sanitizeLog("agent.jsonl", strings.NewReader(string(data)))
	if result.Incomplete || len(result.Events) != len(stages) {
		t.Fatalf("failure phases lost between observer and export: %d, incomplete=%v", len(result.Events), result.Incomplete)
	}
	for i, stage := range stages {
		if result.Events[i].Code != "installed_input_failure_"+stage {
			t.Fatal("failure phase changed in export", stage)
		}
	}
	result = sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-10-16T15:45:34Z","code":"installed_input_failure_PRIVATE"}`+"\n"))
	if len(result.Events) != 0 {
		t.Fatal("dynamic failure code entered the support export")
	}
}
