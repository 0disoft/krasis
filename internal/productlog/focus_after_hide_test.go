package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestPostHideRestoreExportPreservesNumericEvidenceAndRejectsInvalidPresence(t *testing.T) {
	f := &handoff.FocusProfile{Schema: 1, Restore: 3, ResultMeasured: 1, PostHideRestoreForeground: 3, PostHideRestoreAccepted: true, PostHideRestoreMeasured: true, PostHideRestoreMicros: 2200}
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "source", Backend: "none", End: "ok", Focus: f}
	r := record{Time: "2026-10-05T00:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"focus":{`, `"focus":{"targetTitle":"private-marker","savedHandle":"private-handle",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.Focus == nil || *got.Events[0].Handoff.Focus != *f || strings.Contains(string(encoded), "private-") {
		t.Fatal("numeric post-hide evidence lost or unknown fields exported")
	}
	f.PostHideRestoreMeasured = false
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("unmeasured post-hide evidence accepted")
	}
}
