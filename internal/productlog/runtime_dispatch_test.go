package productlog

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

func TestSlowDispatchExportKeepsMatchedStagesAndStripsUnknownData(t *testing.T) {
	s := RuntimeSummary{IntervalMicros: 1, DispatchKind: "idle"}
	s.SlowDispatch[1] = DispatchSpan{Kind: "input", StartedMicros: 1_000_000, DurationMicros: 197000,
		CoordinatorMeasured: true, CoordinatorWaitMicros: 150000, AfterCoordinatorMicros: 45000,
		Injections: 1, InjectionMicros: 2900, InjectionMaxMicros: 2900}
	raw, _ := json.Marshal(record{Time: "2026-10-17T00:00:00Z", Code: "runtime_summary", Runtime: &s})
	raw = []byte(strings.Replace(string(raw), `"kind":"input"`, `"kind":"input","payload":"private-input"`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	if got.Incomplete || len(got.Events) != 1 || *got.Events[0].Runtime != s {
		t.Fatal("lost matched dispatch evidence", got)
	}
	exported, _ := json.Marshal(got)
	if strings.Contains(string(exported), "private-input") || strings.Contains(string(exported), "payload") {
		t.Fatal("unknown data escaped allowlist")
	}
	for _, mutate := range []func(*DispatchSpan){
		func(p *DispatchSpan) { p.Kind = "window title" },
		func(p *DispatchSpan) { p.Kind = "control" }, // wrong array position
		func(p *DispatchSpan) { p.DurationMicros = -1 },
		func(p *DispatchSpan) { p.CoordinatorMeasured = false },
		func(p *DispatchSpan) { p.AfterCoordinatorMicros = 197000 },
		func(p *DispatchSpan) { p.Injections = 0 },
		func(p *DispatchSpan) { p.InjectionMicros = 198000 },
	} {
		invalid := s
		mutate(&invalid.SlowDispatch[1])
		raw, _ = json.Marshal(record{Time: "2026-10-17T00:00:00Z", Code: "runtime_summary", Runtime: &invalid})
		result := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
		if !result.Incomplete || len(result.Events) != 0 {
			t.Fatal("invalid dispatch evidence exported", result)
		}
	}
}

func TestSlowDispatchWorstCaseRetainsExistingFourKiBRecordLimit(t *testing.T) {
	s := RuntimeSummary{IntervalMicros: math.MaxInt64, DispatchKind: "unknown", SlowDispatchDropped: math.MaxUint64}
	fields := reflect.ValueOf(&s).Elem()
	for i := 0; i < fields.NumField(); i++ {
		v := fields.Field(i)
		switch v.Kind() {
		case reflect.Int64:
			v.SetInt(math.MaxInt64)
		case reflect.Uint64:
			v.SetUint(math.MaxUint64)
		case reflect.Bool:
			v.SetBool(true)
		}
	}
	s.GCPauses, s.RunnableDelays = [6]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}, [6]uint64{math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64, math.MaxUint64}
	for i, kind := range []string{"control", "input", "heartbeat", "disconnect", "unknown"} {
		s.SlowDispatch[i] = DispatchSpan{Kind: kind, StartedMicros: math.MaxInt64, DurationMicros: math.MaxInt64,
			ReadMeasured: true, ReadToStartMicros: math.MaxInt64 - 1, Injections: math.MaxUint64,
			InjectionMicros: math.MaxInt64, InjectionMaxMicros: math.MaxInt64}
		if kind == "input" || kind == "heartbeat" {
			s.SlowDispatch[i].CoordinatorMeasured = true
			s.SlowDispatch[i].CoordinatorWaitMicros = math.MaxInt64 / 2
			s.SlowDispatch[i].AfterCoordinatorMicros = math.MaxInt64 / 2
		}
	}
	raw, _ := json.Marshal(record{Time: "2026-10-17T00:00:00.999999999Z", Code: "runtime_summary", Runtime: &s,
		Build: strings.Repeat("a", 64), Run: strings.Repeat("b", 32), Connection: strings.Repeat("c", 64), Sequence: math.MaxUint64, ElapsedMicros: math.MaxInt64})
	if len(raw) > 4096 {
		t.Fatal("slow evidence raised the old runtime record limit", len(raw))
	}
	result := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	if result.Incomplete || len(result.Events) != 1 {
		t.Fatal("bounded worst case lost during export", result)
	}
}
