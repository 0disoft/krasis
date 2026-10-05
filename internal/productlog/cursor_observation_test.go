package productlog

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/application"
)

func TestCursorMismatchObservationSurvivesExportsWithoutPrivateData(t *testing.T) {
	l := &Logger{queue: make(chan record, 1)}
	p := &application.CursorPlacementObservation{Known: 63, Flags: 33}
	l.Injection("entry", &application.CursorTransactionFailure{Operation: "entry", Stage: "mismatch", Cause: errors.New("SECRET coords PIN window path"), Observation: p})
	p.Flags = 0 // queued receipt must not alias caller-owned state
	raw, _ := json.Marshal(<-l.queue)
	result := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	if len(result.Events) != 1 || result.Events[0].Injection.CursorObservation.Flags != 33 {
		t.Fatal("observation lost or aliased")
	}
	document, _ := json.Marshal(map[string]any{"schema": "krasis.support/v1", "created": "2026-10-06T00:00:00Z", "logs": []exportLog{result}})
	clean, err := SanitizeSupport(document)
	if err != nil || !strings.Contains(string(clean), `"cursorObservation":{"known":63,"flags":33}`) || strings.Contains(string(clean), "SECRET") {
		t.Fatal("peer export unsafe or missing", err)
	}
}

func TestCursorObservationCannotAttachToOtherFailureOrForgeSuccess(t *testing.T) {
	for _, value := range []InjectionFailure{
		{Operation: "entry", CursorStage: "mismatch", Certainty: application.InjectionOutcomeUnknown, CursorObservation: &application.CursorPlacementObservation{Known: 3, Flags: 2}},
		{Operation: "entry", CursorStage: "set", Certainty: application.InjectionOutcomeUnknown, CursorObservation: &application.CursorPlacementObservation{Known: 3}},
		{Operation: "rollback", CursorStage: "mismatch", Certainty: application.InjectionOutcomeUnknown, CursorObservation: &application.CursorPlacementObservation{Known: 3}},
		{Operation: "entry", CursorStage: "mismatch", Certainty: application.InjectionOutcomeUnknown, CursorObservation: &application.CursorPlacementObservation{Known: 128}},
	} {
		raw, _ := json.Marshal(record{Code: "injection_failure", Injection: &value})
		result := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
		if len(result.Events) != 0 || !result.Incomplete {
			t.Fatal("forged comparison exported")
		}
	}
	l := &Logger{queue: make(chan record, 1)}
	l.Injection("entry", &application.CursorTransactionFailure{Operation: "entry", Stage: "mismatch", Cause: errors.New("private"), Observation: &application.CursorPlacementObservation{Known: 128}})
	if got := (<-l.queue).Injection; got.CursorStage != "mismatch" || got.CursorObservation != nil {
		t.Fatal("invalid metadata erased original failure")
	}
}
