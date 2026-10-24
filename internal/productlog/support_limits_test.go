package productlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	"github.com/0disoft/krasis/internal/logindiag"
)

func supportArrayDocument(kind string, count int) []byte {
	items := bytes.Repeat([]byte(`{},`), count)
	if count > 0 {
		items = items[:len(items)-1]
	}
	var logs string
	switch kind {
	case "source_exits":
		return []byte(fmt.Sprintf(`{"schema":"krasis.support/v1","created":"2026-10-25T00:00:00Z","logs":[],"loginRuntime":{"source":{"exits":[%s]}}}`, items))
	case "logs":
		logs = string(items)
	case "events":
		logs = `{"source":"agent.jsonl","events":[` + string(items) + `]}`
	case "gc_samples":
		logs = `{"source":"agent.jsonl","events":[{"time":"2026-10-25T00:00:00Z","code":"runtime_gc_trail","gcTrail":{"samples":[` + string(items) + `]}}]}`
	}
	return []byte(fmt.Sprintf(`{"schema":"krasis.support/v1","created":"2026-10-25T00:00:00Z","logs":[%s]}`, logs))
}

func TestRemoteSupportRejectsArrayLimitsBeforeProjection(t *testing.T) {
	for _, tc := range []struct {
		kind  string
		count int
	}{{"events", maxBytes/32 + 1}, {"gc_samples", GCTraceLimit + 1}, {"source_exits", logindiag.MaxSourceExits + 1}} {
		t.Run(tc.kind, func(t *testing.T) {
			data := supportArrayDocument(tc.kind, tc.count)
			if len(data) >= MaxSupportBytes {
				t.Fatal("fixture must fit the existing byte limit")
			}
			if _, err := SanitizeSupport(data); err == nil {
				t.Fatal("oversized diagnostic array accepted")
			}
		})
	}
}

func BenchmarkSupportArrayLimits(b *testing.B) {
	for _, kind := range []string{"events", "gc_samples"} {
		b.Run(kind, func(b *testing.B) {
			data := supportArrayDocument(kind, 100000)
			if len(data) >= MaxSupportBytes {
				b.Fatal("fixture must fit the existing byte limit")
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_, _ = SanitizeSupport(data)
			}
			b.ReportMetric(float64(len(data)), "input-bytes")
		})
	}
}

func TestSupportArrayOverflowDoesNotDecodeTheNextItem(t *testing.T) {
	calls := 0
	_, err := decodeSupportArray([]byte(`[{}, {}, {"untrusted":"suffix"}]`), 2, func(d *json.Decoder) (any, error) {
		calls++
		var value any
		err := d.Decode(&value)
		return value, err
	})
	if err == nil || calls != 2 {
		t.Fatal("array overflow decoded an extra item", calls, err)
	}
}

func TestSupportArrayBoundariesAndNull(t *testing.T) {
	for _, tc := range []struct {
		kind  string
		count int
	}{{"events", maxSupportEvents}, {"gc_samples", GCTraceLimit}, {"source_exits", logindiag.MaxSourceExits}} {
		t.Run(tc.kind, func(t *testing.T) {
			if _, err := SanitizeSupport(supportArrayDocument(tc.kind, tc.count)); err != nil {
				t.Fatal("array at its limit rejected", err)
			}
		})
	}
	for _, data := range []string{`null`, `[]`} {
		var logs boundedSupportLogs
		if err := json.Unmarshal([]byte(data), &logs); err != nil || (logs == nil) != (data == "null") {
			t.Fatal("null/empty array changed", data, logs, err)
		}
	}
}

func TestSupportBoundedGCTrailPreservesFullHistory(t *testing.T) {
	trail := &GCTrail{Schema: 1, TriggerMicros: 7000000, TriggerObservedAt: "2026-10-25T00:00:00Z", Fault: "fault_source_hook_failed", Samples: make([]GCWindow, GCTraceLimit)}
	for i := range trail.Samples {
		trail.Samples[i] = GCWindow{BeginMicros: int64(i*100000 + 1), EndMicros: int64((i+1)*100000 + 1)}
	}
	if !trail.valid() {
		t.Fatal("invalid GC fixture")
	}
	logs := []exportLog{
		{Source: "agent.jsonl", Events: []exportEvent{{Time: "2026-10-25T00:00:00Z", Code: "runtime_gc_trail", GCTrail: trail}}},
		{Source: "agent.previous.jsonl"}, {Source: "control.jsonl"}, {Source: "control.previous.jsonl"},
	}
	doc := struct {
		Schema  string      `json:"schema"`
		Created string      `json:"created"`
		Logs    []exportLog `json:"logs"`
	}{"krasis.support/v1", "2026-10-25T00:00:00Z", logs}
	data, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	clean, err := SanitizeSupport(data)
	doc.Logs = nil
	if err != nil || json.Unmarshal(clean, &doc) != nil || len(doc.Logs) != 4 || len(doc.Logs[0].Events) != 1 {
		t.Fatal("bounded log history was lost", err)
	}
	got := doc.Logs[0].Events[0].GCTrail
	if got == nil || !got.valid() || len(got.Samples) != GCTraceLimit || got.Samples[GCTraceLimit-1] != trail.Samples[GCTraceLimit-1] {
		t.Fatal("bounded GC history was lost", got)
	}
	doc.Logs = append(doc.Logs, exportLog{Source: "agent.jsonl"})
	data, err = json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = SanitizeSupport(data); err == nil {
		t.Fatal("fifth log was accepted")
	}
}

func TestSupportGCSampleBudgetAcrossEvents(t *testing.T) {
	minimum, err := json.Marshal(GCWindow{})
	if err != nil || len(minimum) < 128 {
		t.Fatal("GC sample budget must admit every locally encoded log", len(minimum), err)
	}
	samples := bytes.Repeat([]byte(`{},`), GCTraceLimit)
	event := []byte(`{"gcTrail":{"samples":[` + string(samples[:len(samples)-1]) + `]}},`)
	for _, extra := range []int{0, 1} {
		events := bytes.Repeat(event, maxSupportGCSamplesPerLog/GCTraceLimit+extra)
		data := append([]byte{'['}, events[:len(events)-1]...)
		data = append(data, ']')
		var decoded boundedSupportEvents
		err := json.Unmarshal(data, &decoded)
		if (err != nil) != (extra == 1) {
			t.Fatalf("aggregate GC sample boundary extra=%d: %v", extra, err)
		}
	}
}
