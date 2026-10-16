package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestCursorEntryExportRetainsOnlyValidatedTimingFields(t *testing.T) {
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "ok",
		Entry: &handoff.Vector{Measured: 1 << handoff.Dispatch, Outcome: "ok"}}
	s.Entry.Micros[handoff.Dispatch] = 2000000
	s.Entry.Worker = [3]byte{1, 2, 5}
	s.Entry.WorkerMillis, s.Entry.WorkerMeasured = [3]uint16{0, 1, 1022}, 7
	s.Entry.WorkerApply = &handoff.WorkerApplyProfile{Micros: [2]uint32{1900000, 0}, Measured: 1}
	s.Entry.Measured |= 1 << handoff.ClientResponse
	s.Entry.Micros[handoff.ClientResponse] = 1950000
	s.Entry.ACKRead = &handoff.ACKReadProfile{Micros: [3]uint32{1935000, 1000, 1000}, Measured: 7, Progress: handoff.ACKReadMatched}
	r := record{Time: "2026-10-16T14:14:01Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"entry":{`, `"entry":{"x":123,"pin":"PRIVATE-ENTRY",`, 1))
	data = []byte(strings.Replace(string(data), `"ackRead":{`, `"ackRead":{"pin":"PRIVATE-READ",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Entry == nil || got.Events[0].Handoff.Entry.WorkerApply.Micros[0] != 1900000 {
		t.Fatal("entry detail lost")
	}
	document, _ := json.Marshal(map[string]any{"schema": "krasis.support/v1", "created": "2026-10-16T14:14:01Z", "logs": []exportLog{got}})
	clean, err := SanitizeSupport(document)
	if err != nil || strings.Contains(string(clean), "PRIVATE-ENTRY") || strings.Contains(string(clean), "PRIVATE-READ") || strings.Contains(string(clean), `"x":123`) || !strings.Contains(string(clean), `"ackRead":`) {
		t.Fatal("entry export leaked private fields", err)
	}
	s.Entry.WorkerMeasured = 8
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("invalid entry detail exported")
	}
	s.Entry.WorkerMeasured = 7
	s.Entry.ACKRead.Measured = 8
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("invalid ACK read mask exported")
	}
}
