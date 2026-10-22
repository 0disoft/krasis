package productlog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestPreparationDecisionExportKeepsOnlyValidatedServiceNumbers(t *testing.T) {
	p := &handoff.StartupProfile{Schema: 1, Measured: 3, PreparationDecision: handoff.PreparationExpired}
	p.RecordPreparationWait(132 * time.Millisecond)
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "ok", Startup: p}
	r := record{Time: "2026-10-05T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"startup":{`, `"startup":{"process":"private-marker","session":"private-marker","expiry":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Startup.PreparationDecision != handoff.PreparationExpired ||
		!got.Events[0].Handoff.Startup.PreparationWaitMeasured || got.Events[0].Handoff.Startup.PreparationWaitMicros != 132_000 || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("preparation decision lost or raw identity exported")
	}
	p.PreparationDecision = 255
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unknown preparation reason exported")
	}
}
