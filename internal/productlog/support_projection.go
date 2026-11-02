package productlog

import (
	"bytes"
	"encoding/json"
	"time"
)

const maxLogRecordBytes = 64 * 1024

// Both raw logs and decoded support events use the same validation/projection.
func sanitizeRecord(input record, size int) (exportEvent, bool) {
	if !allowed[input.Code] || size > 4096 && input.Code != "runtime_gc_trail" {
		return exportEvent{}, false
	}
	timestamp, err := time.Parse(time.RFC3339Nano, input.Time)
	if err != nil {
		return exportEvent{}, false
	}
	// Reproject only validated diagnostic identity. Never export arbitrary
	// build strings, process IDs, TLS material, endpoint addresses or raw lines.
	event := exportEvent{Time: timestamp.UTC().Format(time.RFC3339Nano), Code: input.Code}
	if validHex(input.Build, 40) || validHex(input.Build, 64) || input.Build == "development" {
		event.Build = input.Build
		event.BuildDirty = input.BuildDirty
	}
	if validHex(input.Run, 32) && input.Sequence > 0 && input.ElapsedMicros >= 0 {
		event.Run = input.Run
		event.Sequence = input.Sequence
		event.ElapsedMicros = input.ElapsedMicros
	}
	if input.Code != "handoff_profile" && validHex(input.Connection, 64) {
		event.Connection = input.Connection
	}
	if input.Code == "injection_failure" {
		if input.Injection == nil || !input.Injection.valid() {
			return exportEvent{}, false
		}
		event.Injection = input.Injection
	}
	if input.Code == "session_end_detail" {
		if input.SessionEnd == nil || !input.SessionEnd.valid() {
			return exportEvent{}, false
		}
		event.SessionEnd = input.SessionEnd
	}
	if input.Code == "network_dial_observation" {
		if input.Dial == nil || !input.Dial.Valid() {
			return exportEvent{}, false
		}
		event.Dial = input.Dial
	}
	if input.Code == "network_wifi_observation" {
		if input.WiFi == nil || !input.WiFi.Valid() {
			return exportEvent{}, false
		}
		event.WiFi = input.WiFi
	}
	if input.Code == "retry_policy" {
		if input.Retry == nil || !input.Retry.valid() {
			return exportEvent{}, false
		}
		event.Retry = input.Retry
	}
	if input.Code == "runtime_summary" {
		if input.Runtime == nil || !input.Runtime.valid() {
			return exportEvent{}, false
		}
		event.Runtime = input.Runtime
	}
	if input.Code == "runtime_gc_trail" {
		if input.GCTrail == nil || !input.GCTrail.valid() {
			return exportEvent{}, false
		}
		event.GCTrail = input.GCTrail
	}
	if input.Code == "handoff_profile" {
		if input.Handoff == nil || !input.Handoff.Valid() {
			return exportEvent{}, false
		}
		event.Handoff = input.Handoff
	}
	if input.Code == "boundary_summary" {
		if input.Boundary == nil || !input.Boundary.valid() {
			return exportEvent{}, false
		}
		event.Boundary = input.Boundary
	}
	return event, true
}

// Count exact JSONL bytes, including HTML escaping and newlines, while reusing
// one record buffer. Avoid the aggregate JSONL copy and second decode without
// changing scanner limits or the whole-log overflow policy.
func sanitizeSupportLog(input exportLog) (exportLog, error) {
	result := exportLog{Source: input.Source, Incomplete: input.Incomplete, Events: []exportEvent{}}
	var line bytes.Buffer
	encoder := json.NewEncoder(&line)
	total, stopped := 0, false
	for _, item := range input.Events {
		line.Reset()
		if err := encoder.Encode(item); err != nil {
			return exportLog{}, err
		}
		total += line.Len()
		if total > maxBytes {
			continue // Finish encoding to preserve any subsequent encoding error.
		}
		if stopped || line.Len() > maxLogRecordBytes {
			result.Incomplete, stopped = true, true
			continue
		}
		event, ok := sanitizeRecord(record{
			Time: item.Time, Code: item.Code, Build: item.Build, BuildDirty: item.BuildDirty,
			Run: item.Run, Connection: item.Connection, Sequence: item.Sequence, ElapsedMicros: item.ElapsedMicros,
			Boundary: item.Boundary, Injection: item.Injection, Retry: item.Retry, Runtime: item.Runtime,
			GCTrail: item.GCTrail, Handoff: item.Handoff, SessionEnd: item.SessionEnd, WiFi: item.WiFi, Dial: item.Dial,
		}, line.Len()-1)
		if !ok {
			result.Incomplete = true
			continue
		}
		result.Events = append(result.Events, event)
	}
	if total > maxBytes {
		return exportLog{Source: input.Source, Incomplete: true, Events: []exportEvent{}}, nil
	}
	return result, nil
}
