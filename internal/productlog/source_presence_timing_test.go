package productlog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/securefs"
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

func TestSourceHeartbeatFailureTimingPersistsAndExportsOnlyFixedCodes(t *testing.T) {
	path := t.TempDir()
	dir, err := securefs.OpenDirectory(path)
	if err != nil {
		t.Fatal(err)
	}
	logger := newLogger(dir, closer{}, "agent")
	t.Cleanup(logger.Close)
	const prefix = "installed_source_heartbeat_failure_timing_"
	var expected []string
	for _, stage := range []string{"client_lock", "client_authority", "client_write", "client_response", "worker_presence_wait", "worker_presence_native", "worker_reply_gate"} {
		for _, bucket := range []string{"unavailable", "lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"} {
			code := prefix + stage + "_" + bucket
			if !allowed[code] {
				t.Fatal("fixed timing code rejected", code)
			}
		}
	}
	for _, stage := range []string{"client_lock", "client_authority", "client_write", "client_response", "worker_presence_wait", "worker_presence_native", "worker_reply_gate"} {
		code := prefix + stage + "_unavailable"
		expected = append(expected, code)
		logger.Event(code)
	}
	for _, code := range []string{prefix + "PRIVATE_lt1ms", prefix + "client_lock_PRIVATE", prefix + "worker_presence_native_123ms"} {
		logger.Event(code)
		line := `{"time":"2026-10-22T12:00:00Z","code":"` + code + `"}` + "\n"
		if result := sanitizeLog("agent.jsonl", strings.NewReader(line)); len(result.Events) != 0 {
			t.Fatal("arbitrary stage or bucket accepted", code)
		}
	}
	logger.Close()
	data, err := os.ReadFile(filepath.Join(path, "agent.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "PRIVATE") || strings.Contains(string(data), "123ms") {
		t.Fatal("arbitrary data persisted")
	}
	result := sanitizeLog("agent.jsonl", strings.NewReader(string(data)))
	if result.Incomplete || len(result.Events) != len(expected) {
		t.Fatalf("failure timings lost: got %d of %d, incomplete=%t", len(result.Events), len(expected), result.Incomplete)
	}
	for i, code := range expected {
		if result.Events[i].Code != code {
			t.Fatal("failure timing changed during export", code, result.Events[i].Code)
		}
	}
}
