package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestHandoffExchangeTagSurvivesLocalAndPeerExportWithoutRawIdentities(t *testing.T) {
	tag := strings.Repeat("a", 64)
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "source", Backend: "none", End: "ok", ExchangeTag: tag}
	l := &Logger{queue: make(chan record, 1)}
	l.Handoff(s)
	raw, _ := json.Marshal(<-l.queue)
	raw = []byte(strings.Replace(string(raw), `"handoff":{`, `"handoff":{"sessionID":"SECRET","epoch":"SECRET","exporter":"SECRET",`, 1))
	log := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	if log.Incomplete || len(log.Events) != 1 || log.Events[0].Handoff.ExchangeTag != tag {
		t.Fatal("local export lost tag")
	}
	document, _ := json.Marshal(map[string]any{"schema": "krasis.support/v1", "created": "2026-10-06T00:00:00Z", "logs": []exportLog{log}})
	clean, err := SanitizeSupport(document)
	if err != nil || !strings.Contains(string(clean), `"exchangeTag":"`+tag+`"`) || strings.Contains(string(clean), "SECRET") {
		t.Fatal("peer export missing or unsafe", err)
	}
	for _, invalid := range []string{"private-path", strings.Repeat("A", 64), strings.Repeat("a", 65)} {
		s.ExchangeTag = invalid
		raw, _ = json.Marshal(record{Code: "handoff_profile", Handoff: &s})
		bad := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
		if !bad.Incomplete || len(bad.Events) != 0 {
			t.Fatal("invalid tag exported")
		}
	}
}
