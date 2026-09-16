package productlog

import (
	"strings"
	"testing"
)

func TestRemoteSupportReprojectsPrivateFields(t *testing.T) {
	data := []byte(`{"schema":"krasis.support/v1","created":"2026-09-16T00:00:00Z","scope":"SECRET","logs":[{"source":"agent.jsonl","events":[{"time":"2026-09-16T00:00:00Z","code":"agent_ready","secret":"SECRET"},{"time":"2026-09-16T00:00:00Z","code":"SECRET"}]}]}`)
	clean, err := SanitizeSupport(data)
	if err != nil || strings.Contains(string(clean), "SECRET") || !strings.Contains(string(clean), "agent_ready") {
		t.Fatalf("projection: %s %v", clean, err)
	}
	for _, bad := range []string{`{}`, strings.Repeat("x", MaxSupportBytes+1), `{"schema":"krasis.support/v1","created":"2026-09-16T00:00:00Z","logs":[{"source":"../file"}]}`} {
		if _, err := SanitizeSupport([]byte(bad)); err == nil {
			t.Fatal("invalid support accepted")
		}
	}
}
