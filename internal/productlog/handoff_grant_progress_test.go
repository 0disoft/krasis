package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestGrantProgressExportPreservesNumbersAndRejectsUnboundedEvidence(t *testing.T) {
	p := &handoff.GrantProgressProfile{Schema: 1, Micros: [3]uint32{60000, 15000, 5000}, ElapsedMicros: 80000,
		ByteReads: 2, Bytes: 80, Frames: 2, MaxByteGapMicros: 50000, ByteProgressMeasured: true, Complete: true}
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "source", Backend: "none", End: "ok", GrantProgress: p}
	r := record{Time: "2026-10-05T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"grantProgress":{`, `"grantProgress":{"input":"private-marker","address":"private-marker","timestamp":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.GrantProgress == nil || *got.Events[0].Handoff.GrantProgress != *p || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("grant progress projection lost numeric evidence or leaked private fields")
	}
	p.Micros[0] = 80000
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("contradictory phase conservation exported")
	}
}
