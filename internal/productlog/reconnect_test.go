package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestCaptureForegroundStageCodesSurviveExport(t *testing.T) {
	for _, stage := range []string{"before_show", "after_show", "after_request"} {
		code := ActivationCode("source-capture", "capture-activate", "foreground_missing_"+stage)
		if code != "activation_capture_foreground_missing_"+stage {
			t.Fatal("capture stage collapsed to an unknown code", code)
		}
		data, _ := json.Marshal(record{Time: "2026-10-03T00:00:00Z", Code: code})
		exported := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
		if exported.Incomplete || len(exported.Events) != 1 || exported.Events[0].Code != code {
			t.Fatal("capture stage disappeared from support export", code)
		}
	}
}

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

func TestAcquireNetworkReasonsSurviveSanitizedExport(t *testing.T) {
	for _, reason := range []string{"network_refused", "network_unreachable", "network_reset", "network_timeout"} {
		code := ReconnectCode("acquire", "peer-acquire", "network", reason)
		if code != "reconnect_acquire_"+reason {
			t.Fatalf("reason %s mapped to %s", reason, code)
		}
		data, _ := json.Marshal(record{Time: "2026-09-23T00:00:00Z", Code: code})
		exported := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
		if exported.Incomplete || len(exported.Events) != 1 || exported.Events[0].Code != code {
			t.Fatalf("reason dropped: %s", code)
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

func TestSourceOperationFaultCodes(t *testing.T) {
	for _, stage := range []string{"low-level-hook", "raw-mouse", "wheel-capture", "thread-join", "hook-install", "hook-release", "raw-register", "raw-release", "raw-read", "raw-buffer", "raw-route", "message-loop", "pointer-snapshot", "cursor-present", "cursor-restore", "cursor-release"} {
		want := "fault_source_" + strings.ReplaceAll(stage, "-", "_") + "_failed"
		if got := FaultCode("source-hook", stage, "failed"); got != want || !allowed[got] {
			t.Fatalf("stage %s: %s", stage, got)
		}
	}
	if got := FaultCode("source-hook", "private window title", "failed"); got != "fault_session_unknown" {
		t.Fatalf("private stage escaped: %s", got)
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

func TestActivationReasonsAreSanitizedAndExportable(t *testing.T) {
	for descriptor := range reconnectReasons {
		code := ActivationCode(descriptor[0], descriptor[1], descriptor[2])
		data, _ := json.Marshal(record{Time: "2026-09-16T00:00:00Z", Code: code})
		exported := sanitizeLog("agent.jsonl", strings.NewReader(string(data)+"\n"))
		if exported.Incomplete || len(exported.Events) != 1 || exported.Events[0].Code != code {
			t.Fatalf("activation dropped: %s", code)
		}
	}
	if got := ActivationCode("private peer", "private path", "private error"); got != "activation_session_unknown" || !allowed[got] {
		t.Fatalf("unsafe activation code: %s", got)
	}
}
