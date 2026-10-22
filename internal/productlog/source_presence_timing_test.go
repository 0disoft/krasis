package productlog

import (
	"strings"
	"testing"
)

func TestSourceHeartbeatWorkerTimingExportUsesClosedVocabulary(t *testing.T) {
	for _, phase := range []string{"presence_wait", "presence_native", "reply_gate"} {
		code := "installed_source_heartbeat_timing_worker_" + phase + "_ge1s"
		log := sanitizeLog("agent.jsonl", strings.NewReader(`{"time":"2026-10-22T12:00:00Z","code":"`+code+`","input":"private"}`+"\n"))
		if len(log.Events) != 1 || log.Events[0].Code != code {
			t.Fatal(code, log)
		}
	}
	log := sanitizeLog("agent.jsonl", strings.NewReader("{\"time\":\"2026-10-22T12:00:00Z\",\"code\":\"installed_source_heartbeat_timing_worker_PRIVATE_ge1s\"}\n"))
	if len(log.Events) != 0 {
		t.Fatal("private phase accepted")
	}
}
