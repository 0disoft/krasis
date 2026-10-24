package productlog

import (
	"bytes"
	"encoding/json"
	"errors"
)

// A valid event's timestamp and JSON field names alone exceed 32 bytes.
// This deliberately loose bound admits every event in a maxBytes local log.
const maxSupportEvents = maxBytes / 32

// Each locally encoded GCWindow exceeds 128 bytes even with all-zero fields.
// Bound samples across events too, so many short trails cannot multiply memory.
const maxSupportGCSamplesPerLog = maxBytes / 128

// These wire-only types bound dynamic arrays before their elements allocate.
// The ordinary log reader and the JSON exported by this package are unchanged.
type boundedSupportLogs []exportLog
type boundedSupportEvents []exportEvent
type boundedSupportSamples []GCWindow
type boundedSupportGCTrail GCTrail

func (target *boundedSupportLogs) UnmarshalJSON(data []byte) error {
	values, err := decodeSupportArray(data, 4, func(d *json.Decoder) (exportLog, error) {
		var wire struct {
			Source     string               `json:"source"`
			Incomplete bool                 `json:"incomplete"`
			Events     boundedSupportEvents `json:"events"`
		}
		err := d.Decode(&wire)
		return exportLog{Source: wire.Source, Incomplete: wire.Incomplete, Events: wire.Events}, err
	})
	if err == nil {
		*target = values
	}
	return err
}

func (target *boundedSupportEvents) UnmarshalJSON(data []byte) error {
	samples := 0
	values, err := decodeSupportArray(data, maxSupportEvents, func(d *json.Decoder) (exportEvent, error) {
		var wire struct {
			exportEvent
			GCTrail *boundedSupportGCTrail `json:"gcTrail"`
		}
		err := d.Decode(&wire)
		if err == nil && wire.GCTrail != nil {
			if len(wire.GCTrail.Samples) > maxSupportGCSamplesPerLog-samples {
				return exportEvent{}, errors.New("support GC sample budget exceeded")
			}
			samples += len(wire.GCTrail.Samples)
			value := GCTrail(*wire.GCTrail)
			wire.exportEvent.GCTrail = &value
		}
		return wire.exportEvent, err
	})
	if err == nil {
		*target = values
	}
	return err
}

func (target *boundedSupportGCTrail) UnmarshalJSON(data []byte) error {
	var wire struct {
		GCTrail
		Samples boundedSupportSamples `json:"samples"`
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return err
	}
	wire.GCTrail.Samples = wire.Samples
	*target = boundedSupportGCTrail(wire.GCTrail)
	return nil
}

func (target *boundedSupportSamples) UnmarshalJSON(data []byte) error {
	values, err := decodeSupportArray(data, GCTraceLimit, func(d *json.Decoder) (GCWindow, error) {
		var value GCWindow
		err := d.Decode(&value)
		return value, err
	})
	if err == nil {
		*target = values
	}
	return err
}

// encoding/json validates each complete value before invoking UnmarshalJSON.
// Preserve null/empty arrays, but reject overflow before decoding the next item.
func decodeSupportArray[T any](data []byte, limit int, decode func(*json.Decoder) (T, error)) ([]T, error) {
	d := json.NewDecoder(bytes.NewReader(data))
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	if token == nil {
		return nil, nil
	}
	if token != json.Delim('[') {
		return nil, errors.New("invalid support array")
	}
	values := make([]T, 0)
	for d.More() {
		if len(values) == limit {
			return nil, errors.New("support array limit exceeded")
		}
		value, err := decode(d)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
	}
	_, err = d.Token()
	return values, err
}
