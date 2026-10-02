package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestHandoffExportReprojectsFixedSchemaAndDropsIdentity(t *testing.T) {
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "incomplete", Incomplete: true}
	s.Preparation.Micros[handoff.ReadyWait] = 1000
	s.Preparation.Measured = 1 << handoff.ReadyWait
	r := record{Time: "2026-10-03T00:00:00Z", Code: "handoff_profile", Connection: strings.Repeat("a", 64), Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"handoff":{`, `"handoff":{"pin":"secret-marker","path":"private-path",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff == nil || *got.Events[0].Handoff != s {
		t.Fatalf("valid summary lost: %+v", got)
	}
	if strings.Contains(string(encoded), "secret-marker") || strings.Contains(string(encoded), "private-path") || strings.Contains(string(encoded), "connection") {
		t.Fatal("private data exported")
	}
	s.Backend = "secret-marker"
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("arbitrary profile string accepted")
	}
}
