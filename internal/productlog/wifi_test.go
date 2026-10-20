package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestWiFiObservationSanitizesIdentityReasonAndGap(t *testing.T) {
	v := WiFiSummary{State: "connected", Reason: "ap_changed", IdentifiersKnown: true, Interface: "0123456789abcdef", AP: "abcdef0123456789", SampleWindowMicros: 2000000}
	l := &Logger{queue: make(chan record, 1)}
	l.WiFi(v)
	data, _ := json.Marshal(<-l.queue)
	data = []byte(strings.Replace(string(data), `"wifi":{`, `"wifi":{"ssid":"private-ssid","bssid":"private-bssid",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].WiFi == nil || *got.Events[0].WiFi != v {
		t.Fatal("valid WiFi diagnostic lost")
	}
	encoded, _ := json.Marshal(got)
	if strings.Contains(string(encoded), "private-") {
		t.Fatal("raw WLAN data exported")
	}
	for _, change := range []func(*WiFiSummary){
		func(v *WiFiSummary) { v.Interface = "private-guid" }, func(v *WiFiSummary) { v.AP = "01:23:45:67:89:ab" }, func(v *WiFiSummary) { v.State = "private-ssid" },
		func(v *WiFiSummary) { v.Reason = "private-error" }, func(v *WiFiSummary) { v.SampleWindowMicros = -1 }, func(v *WiFiSummary) { v.ObservationGapMicros = -1 },
		func(v *WiFiSummary) { v.IdentifiersKnown = false },
	} {
		bad := v
		change(&bad)
		data, _ := json.Marshal(record{Time: "2026-10-20T18:00:00Z", Code: "network_wifi_observation", WiFi: &bad})
		got := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
		if !got.Incomplete || len(got.Events) != 0 {
			t.Fatal("invalid WLAN diagnostic exported")
		}
	}
}
