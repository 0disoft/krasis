package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestReconnectReasonsSurviveSanitizedExport(t *testing.T) {
	for descriptor, code := range reconnectReasons {
		if got := ReconnectCode("session", descriptor[0], descriptor[1], descriptor[2]); got != code {
			t.Fatalf("got %s want %s", got, code)
		}
		data, _ := json.Marshal(record{Time: "2026-09-15T00:00:00Z", Code: code})
		exported := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
		if exported.Incomplete || len(exported.Events) != 1 || exported.Events[0].Code != code {
			t.Fatalf("reason dropped: %s", code)
		}
	}
	for _, code := range []string{"disconnect_requested", "heartbeat_timeout_observed", ReconnectCode("acquire", "", "", ""), ReconnectCode("session", "source-hook", "runtime", "component_failed")} {
		if !allowed[code] {
			t.Fatalf("code not exportable: %s", code)
		}
	}
}

func TestReconnectReasonsDoNotCopyPrivateDescriptors(t *testing.T) {
	for _, values := range [][4]string{{"private", "private", "private", "private"}, {"session", "private", "read", "remote_eof"}, {"session", "peer-reader", "read", "private"}} {
		if got := ReconnectCode(values[0], values[1], values[2], values[3]); got != "reconnect_session_unknown" {
			t.Fatal("unrecognized descriptor escaped")
		}
	}
}

func TestTerminalFaultReasonsAreSanitizedAndExportable(t *testing.T) {
	for descriptor := range reconnectReasons {
		code := FaultCode(descriptor[0], descriptor[1], descriptor[2])
		data, _ := json.Marshal(record{Time: "2026-09-16T00:00:00Z", Code: code})
		exported := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
		if exported.Incomplete || len(exported.Events) != 1 || exported.Events[0].Code != code {
			t.Fatalf("fault reason dropped: %s", code)
		}
	}
	if got := FaultCode("private peer", "private path", "private error"); got != "fault_session_unknown" || !allowed[got] {
		t.Fatalf("unsafe fault code: %s", got)
	}
}
