package productlog

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestHeartbeatLegRequiresMeasuredMonotonicStagesAndHonestOutcome(t *testing.T) {
	for _, p := range []HeartbeatLeg{
		{}, {Micros: [4]int64{1}, Measured: 1},
		{Micros: [4]int64{1, 2}, Measured: 3},
		{Micros: [4]int64{1, 2, 3}, Measured: 7},
		{Micros: [4]int64{1, 0, 0, 4}, Measured: 9, Outcome: 3},
		{Micros: [4]int64{1, 2, 0, 4}, Measured: 11, Outcome: 2},
		{Micros: [4]int64{1, 2, 3, 4}, Measured: 15, Outcome: 1, LastCompleteMicros: 4},
		{Micros: [4]int64{3}, Measured: 1, LastCompleteMicros: 2},
	} {
		if !p.valid(4, false) {
			t.Fatal("valid stage boundary rejected", p)
		}
	}
	for _, p := range []HeartbeatLeg{
		{Micros: [4]int64{1}, Measured: 0}, {Measured: 2}, {Measured: 16},
		{Micros: [4]int64{1, 2, 0, 4}, Measured: 11, Outcome: 1}, // no write
		{Micros: [4]int64{1, 2, 3, 4}, Measured: 15},             // completed without outcome
		{Micros: [4]int64{1, 2, 5, 4}, Measured: 15, Outcome: 1},
		{Micros: [4]int64{1, 2, 3, 5}, Measured: 15, Outcome: 1},
		{Micros: [4]int64{1, 2, 3}, Measured: 7, Outcome: 3},
		{Micros: [4]int64{1}, Measured: 1, LastCompleteMicros: 2},
		{Micros: [4]int64{1, 2, 3, 4}, Measured: 15, Outcome: 1, LastCompleteMicros: 3},
	} {
		if p.valid(4, false) {
			t.Fatal("invalid or false-success evidence accepted", p)
		}
	}
	if (HeartbeatLeg{Micros: [4]int64{1, 2, 0, 4}, Measured: 11, Outcome: 2}).valid(4, true) {
		t.Fatal("receive could be skipped as inactive")
	}
}

func TestHeartbeatFailureEvidenceExportsWithoutPrivateFieldsOrAliasing(t *testing.T) {
	p := &HeartbeatProgress{Schema: 1, AtMicros: 100, Send: HeartbeatLeg{Micros: [4]int64{10, 20, 30}, Measured: 7}, Receive: HeartbeatLeg{Micros: [4]int64{40, 50, 60, 70}, Measured: 15, Outcome: 1, LastCompleteMicros: 70}, LastSentMicros: 5, LastReceivedMicros: 70}
	s := GCTrail{Schema: 1, TriggerMicros: 100, TriggerObservedAt: "2026-10-19T10:00:00Z", Fault: "fault_heartbeat_timeout", Samples: []GCWindow{{BeginMicros: 1, EndMicros: 101}}, Heartbeat: p}
	l := &Logger{queue: make(chan record, 1)}
	l.GCTrail(s)
	r := <-l.queue
	p.Send.Micros[0] = 99
	if r.GCTrail.Heartbeat.Send.Micros[0] != 10 {
		t.Fatal("mutable heartbeat escaped to log queue")
	}
	r.Time = s.TriggerObservedAt
	raw, _ := json.Marshal(r)
	raw = []byte(strings.Replace(string(raw), `"heartbeat":{`, `"heartbeat":{"pin":"private-marker","session":"private-marker",`, 1))
	got := sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
	encoded, _ := json.Marshal(got)
	if got.Incomplete || len(got.Events) != 1 || got.Events[0].GCTrail.Heartbeat == nil || *got.Events[0].GCTrail.Heartbeat != *r.GCTrail.Heartbeat || strings.Contains(string(encoded), "private-marker") {
		t.Fatal("heartbeat evidence lost or private data leaked")
	}
	for _, mutate := range []func(*HeartbeatProgress){
		func(p *HeartbeatProgress) { p.Schema = 2 },
		func(p *HeartbeatProgress) { p.AtMicros++ },
		func(p *HeartbeatProgress) { p.LastSentMicros = 101 },
		func(p *HeartbeatProgress) { p.Send.Outcome = 1 },
	} {
		bad := *r.GCTrail.Heartbeat
		mutate(&bad)
		copy := *r.GCTrail
		copy.Heartbeat = &bad
		raw, _ := json.Marshal(record{Time: r.Time, Code: r.Code, GCTrail: &copy})
		got = sanitizeLog("agent.jsonl", strings.NewReader(string(raw)+"\n"))
		if !got.Incomplete || len(got.Events) != 0 {
			t.Fatal("invalid heartbeat evidence exported", bad)
		}
	}
}
