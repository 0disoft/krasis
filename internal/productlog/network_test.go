package productlog

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/netobserve"
)

func TestNetworkObservationExportKeepsOnlyFixedCodes(t *testing.T) {
	for _, code := range netobserve.Codes() {
		data := `{"time":"2026-10-20T18:00:00Z","code":"` + code + `","ssid":"private-ssid","address":"private-address","error":"private-error"}` + "\n"
		got := sanitizeLog("agent.jsonl", strings.NewReader(data))
		if got.Incomplete || len(got.Events) != 1 || got.Events[0].Code != code {
			t.Fatal("observation lost from export", code)
		}
		exported, _ := json.Marshal(got)
		if strings.Contains(string(exported), "private-") {
			t.Fatal("raw network data exported")
		}
	}
	bad := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-10-20T18:00:00Z","code":"network_private-ssid"}`+"\n"))
	if !bad.Incomplete || len(bad.Events) != 0 {
		t.Fatal("unregistered network label exported")
	}
}
