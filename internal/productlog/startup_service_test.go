package productlog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestServiceReadinessExportBoundsAndProjectsOnlyFixedNumbers(t *testing.T) {
	p := &handoff.StartupProfile{Schema: 1, Measured: 3}
	p.ServiceReadiness.Record(handoff.ServiceLoginAdmission, 317*time.Millisecond)
	p.ServiceReadiness.Record(handoff.ServiceReadyAuthorization, 0)
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "ok", Startup: p}
	r := record{Time: "2026-10-27T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"serviceReadiness":{`, `"serviceReadiness":{"path":"private-marker","token":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Startup.ServiceReadiness != p.ServiceReadiness || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("service measurements lost or private fields exported")
	}
	p.ServiceReadiness.Micros[handoff.ServiceLoginAdmission] = uint32(handoff.Window.Microseconds() + 1)
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unbounded readiness profile exported")
	}
}
