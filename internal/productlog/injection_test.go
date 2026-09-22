package productlog

import (
	"encoding/json"
	"errors"
	"github.com/0disoft/krasis/internal/application"
	"strings"
	"testing"
)

func TestInjectionCountsSurviveSanitizedExportWithoutInputContent(t *testing.T) {
	for _, operation := range []string{"press", "release", "move", "wheel", "entry", "rollback"} {
		l := &Logger{queue: make(chan record, 1)}
		l.Injection(operation, &application.InjectionError{Certainty: application.InjectionPartiallyApplied, Requested: 3, Inserted: 2, Cause: errors.New("SECRET input and target")})
		raw, _ := json.Marshal(<-l.queue)
		result := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
		if len(result.Events) != 1 || result.Events[0].Injection == nil {
			t.Fatalf("lost injection: %s", raw)
		}
		got := result.Events[0].Injection
		if got.Operation != operation || got.Requested != 3 || got.Inserted != 2 || !got.CountsKnown || strings.Contains(string(raw), "SECRET") {
			t.Fatalf("bad count/projection: %s", raw)
		}
		document, _ := json.Marshal(map[string]any{"schema": "krasis.support/v1", "created": "2026-09-22T00:00:00Z", "logs": []exportLog{result}})
		clean, err := SanitizeSupport(document)
		if err != nil || !strings.Contains(string(clean), `"inserted":2`) {
			t.Fatalf("remote projection: %s %v", clean, err)
		}
	}
}

func TestInvalidInjectionCountsRemainUnknown(t *testing.T) {
	l := &Logger{queue: make(chan record, 1)}
	l.Injection("release", &application.InjectionError{Certainty: application.InjectionPartiallyApplied, Requested: 1, Inserted: 2})
	got := (<-l.queue).Injection
	if got.CountsKnown || got.Certainty != application.InjectionOutcomeUnknown {
		t.Fatalf("impossible count accepted: %+v", got)
	}
	l.Injection("SECRET", errors.New("SECRET"))
	l.Injection("press", nil)
	if len(l.queue) != 0 {
		t.Fatal("invalid operation or success logged")
	}
}
