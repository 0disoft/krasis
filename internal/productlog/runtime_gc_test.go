package productlog

import (
	"encoding/json"
	"math"
	"strings"
	"testing"
)

func TestGCFailureTrailSurvivesSanitizedExportWithoutPrivateFields(t *testing.T) {
	line := `{"time":"2026-10-17T01:00:00Z","code":"runtime_gc_trail","gcTrail":{"schema":1,"triggerMicros":1500000,"triggerObservedAt":"2026-10-17T01:00:00Z","fault":"fault_source_hook_failed","samples":[{"beginMicros":1,"endMicros":1600000,"readMicros":25,"lateMicros":1500000,"known":7,"gcPauses":[0,0,0,0,0,1],"gcStopping":[0,0,0,0,0,1],"runnable":[0,0,1,0,0,0],"PRIVATE":"PIN-and-path"}],"PRIVATE":"credential"}}` + "\n"
	got := sanitizeLog("agent.jsonl", strings.NewReader(line))
	if got.Incomplete || len(got.Events) != 1 {
		t.Fatal("failure-time GC evidence was lost", got)
	}
	raw, _ := json.Marshal(got)
	if strings.Contains(string(raw), "PRIVATE") || strings.Contains(string(raw), "PIN-and-path") || got.Events[0].GCTrail == nil || got.Events[0].GCTrail.Samples[0].GCPauses[5] != 1 {
		t.Fatal("private or missing GC evidence", string(raw))
	}
}

func TestGCFailureTrailBoundedRoundTripAndInvalidWindows(t *testing.T) {
	s := GCTrail{Schema: 1, TriggerMicros: 7000000, TriggerObservedAt: "2026-10-17T01:00:00Z", Fault: "fault_source_hook_failed", Samples: make([]GCWindow, GCTraceLimit)}
	for i := range s.Samples {
		s.Samples[i] = GCWindow{BeginMicros: int64(i*100000 + 1), EndMicros: int64((i+1)*100000 + 1), Known: GCKnownMask, Allocated: math.MaxUint64, Heap: math.MaxUint64, Cycles: math.MaxUint64, AssistMicros: math.MaxUint64, GCCPUMicros: math.MaxUint64}
		for j := 0; j < 6; j++ {
			s.Samples[i].GCPauses[j] = math.MaxUint64
			s.Samples[i].GCStopping[j] = math.MaxUint64
			s.Samples[i].Runnable[j] = math.MaxUint64
		}
	}
	l := &Logger{queue: make(chan record, 1)}
	l.GCTrail(s)
	r := <-l.queue
	s.Samples[0].Heap = 0
	if r.GCTrail.Samples[0].Heap == 0 {
		t.Fatal("queued trail aliased mutable ring")
	}
	r.Time = "2026-10-17T01:00:00Z"
	raw, _ := json.Marshal(r)
	if len(raw) >= 64*1024 {
		t.Fatal("bounded trail exceeds export line limit", len(raw))
	}
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	if got.Incomplete || len(got.Events) != 1 || len(got.Events[0].GCTrail.Samples) != GCTraceLimit {
		t.Fatal("full trail truncated", len(raw))
	}
	for _, mutate := range []func(*GCTrail){
		func(s *GCTrail) { s.Fault = "fault_PRIVATE" },
		func(s *GCTrail) { s.Samples = append(s.Samples, GCWindow{}) },
		func(s *GCTrail) { s.Samples[0].Known = 0 },
		func(s *GCTrail) { s.Samples[0].ReadMicros = 100001 },
		func(s *GCTrail) { s.Samples[1].BeginMicros = 1 },
	} {
		bad := *r.GCTrail
		bad.Samples = append([]GCWindow(nil), r.GCTrail.Samples...)
		mutate(&bad)
		if bad.valid() {
			t.Fatal("invalid trail accepted")
		}
		raw, _ := json.Marshal(record{Time: r.Time, Code: "runtime_gc_trail", GCTrail: &bad})
		out := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
		if !out.Incomplete || len(out.Events) != 0 {
			t.Fatal("invalid trail exported")
		}
	}
}
