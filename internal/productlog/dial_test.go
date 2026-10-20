package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDialExportRejectsPrivateFieldsAndInvalidVocabulary(t *testing.T) {
	v := DialSummary{Phase: "tcp", Family: "ipv4", DNS: "not_applicable", Result: "ok", Route: "selected", Interface: "0123456789abcdef", SuggestedInterface: "0123456789abcdef", LocalChange: "same", EndpointChange: "changed"}
	l := &Logger{queue: make(chan record, 1)}
	l.Dial(v)
	data, _ := json.Marshal(<-l.queue)
	data = []byte(strings.Replace(string(data), `"dial":{`, `"dial":{"address":"SECRET","vpn":"SECRET",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Dial == nil || *got.Events[0].Dial != v {
		t.Fatal("valid diagnostic lost")
	}
	data, _ = json.Marshal(got)
	if strings.Contains(string(data), "SECRET") {
		t.Fatal("private fields exported")
	}
	for _, change := range []func(*DialSummary){func(v *DialSummary) { v.Interface = "192.0.2.1" }, func(v *DialSummary) { v.Route = "VPN" }, func(v *DialSummary) { v.DNS = "private.host" }, func(v *DialSummary) { v.Micros = -1 }, func(v *DialSummary) { v.SuggestedInterface = "" }} {
		bad := v
		change(&bad)
		data, _ := json.Marshal(record{Time: "2026-10-20T18:00:00Z", Code: "network_dial_observation", Dial: &bad})
		got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
		if !got.Incomplete || len(got.Events) != 0 {
			t.Fatal("invalid diagnostic accepted")
		}
	}
}
