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
