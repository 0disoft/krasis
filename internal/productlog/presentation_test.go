package productlog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestPresentationOutcomesSurviveSanitizedExportWithoutNativeData(t *testing.T) {
	l := &Logger{queue: make(chan record, 1)}
	l.Presentation("mark", "timeout", 20*time.Millisecond)
	raw, _ := json.Marshal(<-l.queue)
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	if len(got.Events) != 1 || got.Events[0].Code != "source_presentation_mark_timeout_lt100ms" {
		t.Fatal(string(raw), got)
	}
	l.Presentation("window title", "native error", time.Second)
	if len(l.queue) != 0 {
		t.Fatal("untrusted text entered presentation diagnostics")
	}
}

func TestReturnCursorOutcomesSurviveExport(t *testing.T) {
	l := &Logger{queue: make(chan record, 4)}
	for _, outcome := range []struct{ operation, result string }{
		{"return_snapshot", "timeout"}, {"return_show", "moved"}, {"return_show", "accepted"}, {"return_hide", "failed"},
	} {
		l.Presentation(outcome.operation, outcome.result, time.Millisecond)
		raw, _ := json.Marshal(<-l.queue)
		got := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
		if len(got.Events) != 1 {
			t.Fatal("return cursor outcome removed from export", outcome)
		}
	}
}
