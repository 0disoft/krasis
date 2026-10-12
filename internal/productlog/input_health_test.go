package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInputHealthSupportPreservesFixedFailureAndDropsPrivateFields(t *testing.T) {
	data := []byte(`{"schema":"krasis.support/v1","created":"2026-10-13T00:00:00Z","logs":[],"inputHealth":{"time":"2026-10-13T00:00:00Z","state":"SERVICE_STOPPED","stage":"service_state","win32Error":5,"win32Known":true,"path":"private-path","pin":"secret"}}`)
	clean, err := SanitizeSupport(data)
	if err != nil || strings.Contains(string(clean), "private-path") || strings.Contains(string(clean), "secret") {
		t.Fatal("health observation leaked private data", err)
	}
	var doc struct {
		InputHealth *InputHealthSummary `json:"inputHealth"`
	}
	if json.Unmarshal(clean, &doc) != nil || doc.InputHealth == nil || doc.InputHealth.Stage != "service_state" || !doc.InputHealth.Win32Known {
		t.Fatal("fixed failure lost", string(clean))
	}
	for _, mutate := range []func(*InputHealthSummary){func(v *InputHealthSummary) { v.Stage = "private-path" }, func(v *InputHealthSummary) { v.State = "secret" }, func(v *InputHealthSummary) { v.Win32Known = false }, func(v *InputHealthSummary) { v.Time = "invalid" }} {
		v := *doc.InputHealth
		mutate(&v)
		if v.valid() {
			t.Fatal("invalid health observation accepted", v)
		}
		bad, _ := json.Marshal(map[string]any{"schema": "krasis.support/v1", "created": "2026-10-13T00:00:00Z", "logs": []exportLog{}, "inputHealth": v})
		output, err := SanitizeSupport(bad)
		if err != nil || strings.Contains(string(output), `"inputHealth":`) || !strings.Contains(string(output), `"inputHealthIncomplete":true`) {
			t.Fatal("invalid observation exported", err, string(output))
		}
	}
}
