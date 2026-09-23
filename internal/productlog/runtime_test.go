package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestRuntimeExportPreservesBoundedNumericEvidence(t *testing.T) {
	l := &Logger{queue: make(chan record, 1)}
	s := RuntimeSummary{IntervalMicros: 5000000, HeapBytes: 1000, AllocatedBytes: 500, QueueCoalesced: 7, FrameReadBytes: 19, LastReadByteAgeMicros: 1000, ReadByteObserved: true, DispatchKind: "input", DispatchAgeMicros: 700000, CPUKnown: true, CPUMicros: 500, WorkingSetKnown: true, WorkingSetBytes: 5000, GCPauses: [6]uint64{1, 2}}
	l.Runtime(s)
	raw, _ := json.Marshal(<-l.queue)
	result := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	if len(result.Events) != 1 || result.Events[0].Runtime == nil || *result.Events[0].Runtime != s {
		t.Fatalf("lost evidence: %s", raw)
	}
	for _, invalid := range []RuntimeSummary{{IntervalMicros: 1, DispatchKind: "sensitive free text"}, {IntervalMicros: 1, DispatchKind: "input", WorkingSetBytes: 1}, {IntervalMicros: 1, DispatchKind: "idle", TimerLateMicros: -1}, {IntervalMicros: 1, DispatchKind: "idle", QueueMaxResidenceMicros: -1}} {
		l.Runtime(invalid)
		if len(l.queue) != 0 {
			t.Fatal("invalid summary queued")
		}
		raw, _ = json.Marshal(record{Time: "2026-09-22T00:00:00Z", Code: "runtime_summary", Runtime: &invalid})
		if got := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n")); len(got.Events) != 0 || !got.Incomplete {
			t.Fatalf("invalid summary exported=%+v", got)
		}
	}
}
