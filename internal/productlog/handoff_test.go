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
	s.First.Worker = [3]byte{1, 4, 2}
	s.First.WorkerMillis = [3]uint16{0, 197, 4}
	s.First.WorkerMeasured = 7
	s.First.AuthorityMicros = [3]uint32{0, 4, 15000}
	s.First.AuthorityMeasured = 7
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
	s.First.WorkerMeasured = 1
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unobserved worker milliseconds exported")
	}
	s.First.WorkerMeasured = 7
	s.First.AuthorityMeasured = 1
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unobserved authority microseconds exported")
	}
	s.First.AuthorityMeasured = 7
	s.Backend = "secret-marker"
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("arbitrary profile string accepted")
	}
}

func TestStartupExportReprojectsNumericSchemaAndRejectsInvalidValues(t *testing.T) {
	profile := &handoff.StartupProfile{Schema: 1, Measured: 1}
	profile.Micros[handoff.ServiceLaunch] = 1200
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "ok", Startup: profile}
	r := record{Time: "2026-10-03T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"startup":{`, `"startup":{"pin":"secret-marker","path":"private-path",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Startup == nil || *got.Events[0].Handoff.Startup != *profile {
		t.Fatal("validated startup diagnostics lost")
	}
	if strings.Contains(string(encoded), "secret-marker") || strings.Contains(string(encoded), "private-path") {
		t.Fatal("private startup fields exported")
	}
	profile.Micros[handoff.WorkerIdentity] = 1
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unmeasured startup value accepted by export")
	}
}
