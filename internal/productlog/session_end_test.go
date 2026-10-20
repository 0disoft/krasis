package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/failurecode"
)

func TestSessionEndExportKeepsFixedTriggerNativeAndLifecycleEvidence(t *testing.T) {
	l := &Logger{queue: make(chan record, 4)}
	l.Event("lifecycle_session_lock")
	l.Event("installed_source_capture_protected_ready")
	<-l.queue
	<-l.queue
	s := SessionEndSummary{ObservedAt: "2026-10-15T09:14:20Z", Trigger: true, Component: "peer-liveness", Fault: failurecode.FaultCode("peer-writer", "write-deadline", "failed"), Closed: true, NativeError: 5, NativeKnown: true}
	s.NetworkError, s.NetworkPolicy = "network_access_denied", "unknown"
	l.SessionEnd(s)
	raw, _ := json.Marshal(<-l.queue)
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].SessionEnd == nil {
		t.Fatalf("lost evidence: %s", raw)
	}
	v := got.Events[0].SessionEnd
	if !v.Trigger || v.NativeError != 5 || v.LastLifecycle != "session_lock" || v.CaptureMode != "protected" || v.ObservedAt != s.ObservedAt {
		t.Fatalf("wrong evidence: %+v", v)
	}
	if v.NetworkError != "network_access_denied" || v.NetworkPolicy != "unknown" {
		t.Fatal("network evidence lost", v)
	}
	for _, change := range []func(*SessionEndSummary){
		func(v *SessionEndSummary) { v.Component = "private window title" },
		func(v *SessionEndSummary) { v.Fault = "fault_private_secret" },
		func(v *SessionEndSummary) { v.NativeKnown = false },
		func(v *SessionEndSummary) { v.ObservedAt = "invalid" },
		func(v *SessionEndSummary) { v.LastLifecycle = "private desktop name" },
		func(v *SessionEndSummary) { v.CaptureMode = "private process path" },
		func(v *SessionEndSummary) { v.NetworkPolicy = "blocked" },
		func(v *SessionEndSummary) { v.NetworkError = "SECRET firewall rule" },
	} {
		invalid := *v
		change(&invalid)
		raw, _ := json.Marshal(record{Time: s.ObservedAt, Code: "session_end_detail", SessionEnd: &invalid})
		got := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
		if !got.Incomplete || len(got.Events) != 0 {
			t.Fatalf("unsafe evidence exported: %s", raw)
		}
	}
}
