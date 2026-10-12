package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestBoundaryPhaseExportRetainsResultWithoutInputDetails(t *testing.T) {
	for _, phase := range []string{"dropped_not_ready", "blocked_menu", "blocked_capture", "blocked_pressed", "blocked_move_size", "admission_failed", "admitted", "request_written", "request_unknown"} {
		code := "boundary_" + phase
		input := `{"time":"2026-10-12T00:00:00Z","code":"` + code + `","key":"private-marker","window":"private-marker"}` + "\n"
		result := sanitizeLog("agent.jsonl", strings.NewReader(input))
		data, err := json.Marshal(result)
		if err != nil || result.Incomplete || len(result.Events) != 1 || result.Events[0].Code != code || strings.Contains(string(data), "private-marker") {
			t.Fatal("phase lost or private fields leaked", code, err)
		}
	}
}
