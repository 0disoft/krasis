package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestReadPredecessorExportProjectsBoundedNumbersWithoutInputContent(t *testing.T) {
	p := &handoff.ReadPredecessorProfile{Schema: 1, Kind: 2, Micros: [3]uint32{80000, 1000, 5000}}
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "ok", ReadPredecessor: p}
	r := record{Time: "2026-10-19T10:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"readPredecessor":{`, `"readPredecessor":{"input":"private-marker","timestamp":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.ReadPredecessor == nil || *got.Events[0].Handoff.ReadPredecessor != *p || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("predecessor projection lost evidence or leaked private data")
	}
	for _, mutate := range []func(){
		func() { s.Role, s.Backend = "source", "none" },
		func() { s.Role, s.Backend = "receiver", "installed"; p.Kind = 4 },
		func() { p.Kind = 2; p.Micros[0] = uint32(handoff.Window.Microseconds()) + 1 },
	} {
		mutate()
		data, _ = json.Marshal(r)
		got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
		if !got.Incomplete || len(got.Events) != 0 {
			t.Fatal("invalid predecessor evidence exported")
		}
	}
}
