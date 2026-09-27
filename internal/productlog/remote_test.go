package productlog

import (
	"encoding/json"
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

func TestRemoteSupportAcceptsTwoExtendedLogs(t *testing.T) {
	events := make([]exportEvent, 10000)
	for i := range events {
		events[i] = exportEvent{Time: "2026-09-16T00:00:00Z", Code: "process_started"}
	}
	input, err := json.Marshal(struct {
		Schema  string      `json:"schema"`
		Created string      `json:"created"`
		Logs    []exportLog `json:"logs"`
	}{"krasis.support/v1", "2026-09-16T00:00:00Z", []exportLog{
		{Source: "agent.previous.jsonl", Events: events},
		{Source: "agent.jsonl", Events: events},
	}})
	if err != nil || len(input) <= 1<<20 {
		t.Fatalf("fixture must exceed the old 1 MiB limit: %d bytes, %v", len(input), err)
	}
	clean, err := SanitizeSupport(input)
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Logs []exportLog `json:"logs"`
	}
	if err := json.Unmarshal(clean, &result); err != nil || len(result.Logs) != 2 || len(result.Logs[0].Events) != len(events) || len(result.Logs[1].Events) != len(events) {
		t.Fatalf("extended history lost: %v", err)
	}
}
