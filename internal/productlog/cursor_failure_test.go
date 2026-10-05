package productlog

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/application"
)

func TestCursorStageSurvivesLocalAndPeerSanitization(t *testing.T) {
	for operation, stages := range map[string][]string{
		"entry":    {"context", "state", "snapshot", "geometry", "set", "verify", "mismatch"},
		"rollback": {"context", "state", "unverified", "inspect", "restore", "verify", "mismatch"},
	} {
		for _, stage := range stages {
			l := &Logger{queue: make(chan record, 1)}
			l.Injection(operation, &application.CursorTransactionFailure{Operation: operation, Stage: stage, Cause: errors.New("SECRET coordinates and window")})
			raw, err := json.Marshal(<-l.queue)
			if err != nil || strings.Contains(string(raw), "SECRET") {
				t.Fatal("unsafe receipt", err)
			}
			result := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
			if len(result.Events) != 1 || result.Events[0].Injection.CursorStage != stage || result.Events[0].Injection.CountsKnown {
				t.Fatal("stage lost or outcome invented")
			}
			document, _ := json.Marshal(map[string]any{"schema": "krasis.support/v1", "created": "2026-10-05T00:00:00Z", "logs": []exportLog{result}})
			clean, err := SanitizeSupport(document)
			if err != nil || !strings.Contains(string(clean), `"cursorStage":"`+stage+`"`) {
				t.Fatal("peer export lost stage", err)
			}
		}
	}
}

func TestCursorStageCannotInjectExportDataOrOverwriteAnotherOperation(t *testing.T) {
	for _, failure := range []*application.CursorTransactionFailure{
		{Operation: "entry", Stage: "SECRET", Cause: errors.New("SECRET")},
		{Operation: "rollback", Stage: "restore", Cause: errors.New("SECRET")},
	} {
		l := &Logger{queue: make(chan record, 1)}
		l.Injection("entry", failure)
		if got := (<-l.queue).Injection; got.CursorStage != "" {
			t.Fatal("invalid stage attached", got)
		}
	}
	for _, value := range []InjectionFailure{
		{Operation: "entry", CursorStage: "SECRET", Certainty: application.InjectionOutcomeUnknown},
		{Operation: "rollback", CursorStage: "snapshot", Certainty: application.InjectionOutcomeUnknown},
		{Operation: "move", CursorStage: "set", Certainty: application.InjectionOutcomeUnknown},
	} {
		raw, _ := json.Marshal(record{Code: "injection_failure", Injection: &value})
		result := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
		if len(result.Events) != 0 || !result.Incomplete {
			t.Fatal("forged cursor stage exported")
		}
	}
}
