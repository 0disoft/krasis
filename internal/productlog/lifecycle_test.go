package productlog

import (
	"strings"
	"testing"
)

func TestLifecycleCodesSurviveExportWithoutPrivateContext(t *testing.T) {
	for _, event := range []string{"suspend", "resume", "session_lock", "session_disconnect", "display_change", "logoff", "shutdown", "monitor_failed"} {
		line := `{"time":"2026-09-22T00:00:00Z","code":"lifecycle_` + event + `","desktop":"SECRET"}` + "\n"
		result := sanitizeLog("agent.jsonl", strings.NewReader(line))
		if len(result.Events) != 1 || result.Events[0].Code != "lifecycle_"+event {
			t.Fatal(event)
		}
	}
}
