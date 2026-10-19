package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestInputACKPolicyExportKeepsAcceptanceSeparateFromDelivery(t *testing.T) {
	for _, code := range []string{"tcp_ack_every_packet_requested", "tcp_ack_frequency_unavailable", "tcp_ack_frequency_not_tcp", "tcp_ack_frequency_unsupported"} {
		result := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-10-19T12:40:51Z","code":"`+code+`","endpoint":"private"}`+"\n"))
		encoded, _ := json.Marshal(result)
		if result.Incomplete || len(result.Events) != 1 || strings.Contains(string(encoded), "private") {
			t.Fatal("ACK policy code lost or raw details exported", code)
		}
	}
	result := sanitizeLog("agent.jsonl", strings.NewReader("{\"time\":\"2026-10-19T12:40:51Z\",\"code\":\"tcp_ack_private\"}\n"))
	if !result.Incomplete || len(result.Events) != 0 {
		t.Fatal("unknown ACK policy code exported")
	}
}
