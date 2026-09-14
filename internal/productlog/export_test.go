package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestExportReprojectsOnlyAllowedFields(t *testing.T) {
	input := `{"time":"2026-09-14T10:00:00+09:00","code":"agent_ready","build":"private-path","pid":123,"pin":"secret-marker"}` + "\n"
	got := sanitizeLog("agent.jsonl", strings.NewReader(input))
	data, err := json.Marshal(got)
	if err != nil || got.Incomplete || len(got.Events) != 1 {
		t.Fatalf("unexpected result: %+v %v", got, err)
	}
	if strings.Contains(string(data), "private-path") || strings.Contains(string(data), "secret-marker") || strings.Contains(string(data), "pid") {
		t.Fatal("private fields leaked")
	}
	if got.Events[0].Time != "2026-09-14T01:00:00Z" {
		t.Fatal("timestamp was not normalized")
	}
}

func TestExportRejectsMalformedOversizedAndPartialRecords(t *testing.T) {
	for name, data := range map[string]string{
		"unknown":      `{"time":"2026-09-14T00:00:00Z","code":"secret-marker"}` + "\n",
		"invalid time": `{"time":"secret-marker","code":"agent_ready"}` + "\n",
		"partial":      `{"time":"2026-09-14T00:00:00Z","code":"agent_ready"}`,
		"large":        strings.Repeat("x", maxBytes+1),
		"long line":    strings.Repeat("x", 5000) + "\n",
	} {
		t.Run(name, func(t *testing.T) {
			got := sanitizeLog("agent.jsonl", strings.NewReader(data))
			if !got.Incomplete || len(got.Events) != 0 {
				t.Fatal("invalid record exported")
			}
		})
	}
}

func TestBoundaryExportValidationAndPrivacy(t *testing.T) {
	s := BoundarySummary{Width: 1536, Height: 864, PhysicalWidth: 1920, PhysicalHeight: 1080, DPIAwareness: 0, Edge: "RIGHT", Rearm: 32, Candidates: 2, LastOffset: 0, Complete: true}
	r := record{Time: "2026-09-14T00:00:00Z", Code: "boundary_summary", Boundary: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"boundary":{`, `"boundary":{"pin":"secret-marker","path":"private-path",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Boundary == nil || *got.Events[0].Boundary != s {
		t.Fatalf("summary lost: %+v", got)
	}
	exported, _ := json.Marshal(got)
	if strings.Contains(string(exported), "secret-marker") || strings.Contains(string(exported), "private-path") {
		t.Fatal("extra fields leaked")
	}
	for _, mutate := range []func(*BoundarySummary){
		func(s *BoundarySummary) { s.Edge = "private-path" },
		func(s *BoundarySummary) { s.Width = -1 },
		func(s *BoundarySummary) { s.DPIAwareness = 3 },
		func(s *BoundarySummary) { s.Rearm = s.Width },
	} {
		bad := s
		mutate(&bad)
		r.Boundary = &bad
		data, _ := json.Marshal(r)
		got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
		if !got.Incomplete || len(got.Events) != 0 {
			t.Fatal("invalid geometry exported")
		}
	}
}

func TestBoundaryLoggerCopiesSnapshotAndDropsWhenFull(t *testing.T) {
	l := &Logger{queue: make(chan record, 1)}
	s := BoundarySummary{Width: 1920, Height: 1080, Edge: "RIGHT", Rearm: 32}
	l.Boundary(s)
	s.Width = 100
	l.Boundary(s)
	if !l.Failed() || (<-l.queue).Boundary.Width != 1920 {
		t.Fatal("queue contract violated")
	}
	var absent *Logger
	absent.Boundary(s)
}
