package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestPipeOpenExportKeepsBoundedNumbersAndDropsPrivateFields(t *testing.T) {
	p := &handoff.PipeOpenProfile{Schema: 1, Attempts: 3, Busy: 1, Missing: 1, OpenMicros: 1000, WaitMicros: 6000}
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "ok", PipeOpen: p}
	s.Preparation.Measured = 1 << handoff.PipeOpen
	s.Preparation.Micros[handoff.PipeOpen] = 8000
	r := record{Time: "2026-10-19T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"pipeOpen":{`, `"pipeOpen":{"path":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.PipeOpen == nil || *got.Events[0].Handoff.PipeOpen != *p || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("numeric evidence or privacy projection failed")
	}
	p.WaitMicros = 9000
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("impossible pipe partition exported")
	}
}
