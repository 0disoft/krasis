package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestTCPInfoExportRetainsOnlyValidFixedNumericEvidence(t *testing.T) {
	p := &handoff.TCPInfoProfile{Schema: 1, RTTMicros: [2]uint32{1000, 2000}, MinRTTMicros: [2]uint32{500, 500},
		CounterDeltasKnown: true, RetransmittedBytes: 400, TimeoutEpisodes: 1}
	s := handoff.Summary{Schema: 1, Attempt: 1, Role: "source", Backend: "none", End: "ok", TCPInfo: p}
	r := record{Time: "2026-10-19T11:00:00Z", Code: "handoff_profile", Handoff: &s}
	data, _ := json.Marshal(r)
	data = []byte(strings.Replace(string(data), `"tcpInfo":{`, `"tcpInfo":{"address":"private-marker","rawSocket":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].Handoff.TCPInfo == nil || *got.Events[0].Handoff.TCPInfo != *p || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("numeric TCP projection lost observations or leaked raw fields")
	}
	s.Role, s.Backend = "receiver", "installed"
	data, _ = json.Marshal(r)
	got = sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if !got.Incomplete || len(got.Events) != 0 {
		t.Fatal("wrong-role TCP diagnostics exported")
	}
}
