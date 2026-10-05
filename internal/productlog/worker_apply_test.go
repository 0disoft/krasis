package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestWorkerApplyLocalAndPeerExportOnlyBoundedTimings(t *testing.T) {
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "receiver", Backend: "installed", End: "ok"}
	s.First.WorkerApply = &handoff.WorkerApplyProfile{Micros: [2]uint32{31001, 0}, Measured: 3, SubmissionMicros: 30000, SubmissionCalls: 1}
	r := record{Time: "2026-10-06T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"workerApply":{`, `"workerApply":{"private":"SECRET",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.First.WorkerApply.Micros[0] != 31001 || got.Events[0].Handoff.First.WorkerApply.SubmissionMicros != 30000 {
		t.Fatal("detail lost")
	}
	document, _ := json.Marshal(map[string]any{"schema": "krasis.support/v1", "created": "2026-10-06T00:00:00Z", "logs": []exportLog{got}})
	clean, err := SanitizeSupport(document)
	if err != nil || strings.Contains(string(clean), "SECRET") || !strings.Contains(string(clean), `"submissionMicros":30000`) {
		t.Fatal("unsafe peer export", err)
	}
	s.First.WorkerApply.Measured = 4
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("invalid apply detail exported")
	}
}
