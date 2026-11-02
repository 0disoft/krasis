package productlog

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

func supportLogThroughJSONL(input exportLog) (exportLog, error) {
	var lines bytes.Buffer
	for _, event := range input.Events {
		if err := json.NewEncoder(&lines).Encode(event); err != nil {
			return exportLog{}, err
		}
	}
	result := sanitizeLog(input.Source, &lines)
	result.Incomplete = result.Incomplete || input.Incomplete
	return result, nil
}

func checkSupportProjection(t *testing.T, input exportLog) exportLog {
	t.Helper()
	before, err := json.Marshal(input)
	if err != nil {
		t.Fatal(err)
	}
	want, err := supportLogThroughJSONL(input)
	if err != nil {
		t.Fatal(err)
	}
	got, err := sanitizeSupportLog(input)
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("projection differs: events=%d/%d incomplete=%v/%v error=%v", len(got.Events), len(want.Events), got.Incomplete, want.Incomplete, err)
	}
	after, err := json.Marshal(input)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("projection mutated caller-owned events")
	}
	return got
}

// Exact encoded length includes Encoder's terminating newline. Padding in an
// untrusted build field must affect byte budgets even though it is not exported.
func paddedSupportEvent(t *testing.T, code string, size int) exportEvent {
	t.Helper()
	event := exportEvent{Time: "2026-11-02T00:00:00Z", Code: code, Build: "x"}
	if code == "runtime_gc_trail" {
		event.GCTrail = &GCTrail{Schema: 1, TriggerMicros: 2, TriggerObservedAt: event.Time, Fault: "fault_source_hook_failed", Samples: []GCWindow{{BeginMicros: 1, EndMicros: 2}}}
		if !event.GCTrail.valid() {
			t.Fatal("invalid GC fixture")
		}
	}
	data, err := json.Marshal(event)
	if err != nil || size < len(data) {
		t.Fatal("invalid target size", size, err)
	}
	event.Build = strings.Repeat("x", size-len(data))
	data, _ = json.Marshal(event)
	if len(data)+1 != size {
		t.Fatal("fixture byte count differs from target")
	}
	return event
}

func TestSupportProjectionPreservesRecordSizeBoundaries(t *testing.T) {
	good := exportEvent{Time: "2026-11-02T00:00:00Z", Code: "agent_ready"}
	t.Run("HTML escaping counts toward the byte limit", func(t *testing.T) {
		middle := good
		middle.Build = strings.Repeat("<", 700) // Encoder escapes each byte as six bytes.
		got := checkSupportProjection(t, exportLog{Source: "agent.jsonl", Events: []exportEvent{good, middle, good}})
		if len(got.Events) != 2 || !got.Incomplete {
			t.Fatal("encoded byte limit was replaced with a string length limit")
		}
	})
	for _, code := range []string{"agent_ready", "runtime_gc_trail"} {
		for _, size := range []int{4096, 4097, 4098, maxLogRecordBytes - 1, maxLogRecordBytes, maxLogRecordBytes + 1} {
			t.Run(fmt.Sprintf("%s/bytes=%d", code, size), func(t *testing.T) {
				middle := paddedSupportEvent(t, code, size)
				got := checkSupportProjection(t, exportLog{Source: "agent.jsonl", Events: []exportEvent{good, middle, good}})
				count := 3
				if size > maxLogRecordBytes {
					count = 1 // Scanner stops here and does not resume at the suffix.
				} else if size > 4097 && code != "runtime_gc_trail" {
					count = 2
				}
				if len(got.Events) != count || got.Incomplete != (count != 3) {
					t.Fatal("record boundary behavior changed", len(got.Events), got.Incomplete)
				}
			})
		}
	}
}

func TestSupportProjectionPreservesWholeLogLimit(t *testing.T) {
	for _, delta := range []int{-1, 0, 1} {
		t.Run(fmt.Sprint(delta), func(t *testing.T) {
			events := make([]exportEvent, 32)
			for i := range events {
				size := maxBytes / len(events)
				if i == len(events)-1 {
					size += delta
				}
				events[i] = paddedSupportEvent(t, "runtime_gc_trail", size)
			}
			got := checkSupportProjection(t, exportLog{Source: "agent.jsonl", Events: events})
			if delta > 0 {
				if len(got.Events) != 0 || !got.Incomplete {
					t.Fatal("oversized log retained a partial event list")
				}
			} else if len(got.Events) != len(events) || got.Incomplete {
				t.Fatal("bounded log rejected", len(got.Events), got.Incomplete)
			}
		})
	}
}

func TestSupportProjectionPreservesPrivacyAndDuplicateOrder(t *testing.T) {
	good := exportEvent{Time: "2026-11-02T09:00:00+09:00", Code: "process_started", Build: "development", BuildDirty: true,
		Run: strings.Repeat("a", 32), Connection: strings.Repeat("b", 64), Sequence: 3, ElapsedMicros: 1}
	events := []exportEvent{good, good,
		{Time: good.Time, Code: "agent_ready", Build: "SECRET <>&\n", Run: "SECRET", Connection: "SECRET", Sequence: 1},
		{Time: "SECRET", Code: "agent_ready"}, {Time: good.Time, Code: "SECRET"},
	}
	for _, code := range []string{"injection_failure", "session_end_detail", "network_dial_observation", "network_wifi_observation", "retry_policy", "runtime_summary", "runtime_gc_trail", "handoff_profile", "boundary_summary"} {
		events = append(events, exportEvent{Time: good.Time, Code: code}) // Missing required typed payload.
	}
	got := checkSupportProjection(t, exportLog{Source: "agent.jsonl", Events: events})
	data, _ := json.Marshal(got)
	if len(got.Events) != 3 || !got.Incomplete || strings.Contains(string(data), "SECRET") || !reflect.DeepEqual(got.Events[0], got.Events[1]) {
		t.Fatal("privacy, duplicate retention or event ordering changed")
	}
	checkSupportProjection(t, exportLog{Source: "agent.jsonl", Incomplete: true})
	checkSupportProjection(t, exportLog{Source: "agent.jsonl", Events: []exportEvent{}})
}

func BenchmarkSanitizeSupportProjection(b *testing.B) {
	for _, count := range []int{100, 10000} {
		b.Run(fmt.Sprintf("events=%d", count), func(b *testing.B) {
			events := make([]exportEvent, count)
			for i := range events {
				events[i] = exportEvent{Time: "2026-11-02T00:00:00Z", Code: "process_started"}
			}
			data, err := json.Marshal(struct {
				Schema  string      `json:"schema"`
				Created string      `json:"created"`
				Logs    []exportLog `json:"logs"`
			}{"krasis.support/v1", "2026-11-02T00:00:00Z", []exportLog{{Source: "agent.jsonl", Events: events}}})
			if err != nil {
				b.Fatal(err)
			}
			b.SetBytes(int64(len(data)))
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := SanitizeSupport(data); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
