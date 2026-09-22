package productlog

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDiagnosticExportKeepsValidatedBuildAndClockIndependentOrder(t *testing.T) {
	connection := strings.Repeat("a", 64)
	logger := &Logger{queue: make(chan record, 2), build: strings.Repeat("b", 40), run: strings.Repeat("c", 32), started: time.Now()}
	logger.Connection(connection)
	logger.Event("reconnect_write_io_timeout")
	logger.Event("reconnect_after_local_interrupt")
	first, second := <-logger.queue, <-logger.queue
	// Wall-clock adjustment must not reorder events within the same process.
	second.Time = "2000-01-01T00:00:00Z"
	var lines strings.Builder
	for _, record := range []record{first, second} {
		data, _ := json.Marshal(record)
		lines.Write(data)
		lines.WriteByte('\n')
	}
	log := sanitizeLog("agent.jsonl", strings.NewReader(lines.String()))
	if len(log.Events) != 2 || log.Events[0].Sequence != 1 || log.Events[1].Sequence != 2 || log.Events[0].Connection != connection || log.Events[1].Run != logger.run || log.Events[0].Build != logger.build {
		t.Fatalf("lost provenance: %+v", log)
	}
	data, _ := json.Marshal(map[string]any{"schema": "krasis.support/v1", "created": "2026-09-22T00:00:00Z", "logs": []exportLog{log}})
	clean, err := SanitizeSupport(data)
	if err != nil || !strings.Contains(string(clean), connection) || !strings.Contains(string(clean), logger.build) {
		t.Fatalf("remote projection lost provenance: %s, %v", clean, err)
	}
}

func TestDiagnosticIdentityNeverExportsUnvalidatedTextOrProcessID(t *testing.T) {
	input := `{"time":"2026-09-22T00:00:00Z","code":"agent_ready","build":"SECRET-build","run":"SECRET-run","connection":"SECRET-host","sequence":1,"elapsedMicros":-1,"pid":123,"key":"SECRET-key"}` + "\n"
	log := sanitizeLog("agent.jsonl", strings.NewReader(input))
	data, _ := json.Marshal(log)
	if len(log.Events) != 1 || strings.Contains(string(data), "SECRET") || strings.Contains(string(data), "pid") || log.Events[0].Sequence != 0 {
		t.Fatalf("untrusted identity exported: %s", data)
	}
}
