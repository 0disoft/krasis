package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestCadenceExportKeepsNumericEvidenceAndRejectsContradictions(t *testing.T) {
	p := &handoff.CadenceProfile{Schema: 1, Intervals: 3, Histogram: [6]uint32{1, 1, 0, 0, 0, 1}, MaxGapMicros: 150000, PreviousDispatchMicros: 2000, BetweenDispatchMicros: 148000}
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "source", Backend: "none", End: "ok", Cadence: p}
	r := record{Time: "2026-10-19T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"cadence":{`, `"cadence":{"input":"private-marker","address":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Cadence == nil || *got.Events[0].Handoff.Cadence != *p || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("bounded cadence was lost or unknown input fields survived projection")
	}
	p.PreviousDispatchMicros = 200000
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("contradictory cadence became valid exported evidence")
	}
}
