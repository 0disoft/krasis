package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"
)

func TestTimingReplyPreservesLegacyFormatAndRejectsInvalidBuckets(t *testing.T) {
	for _, buckets := range [][3]byte{{}, {1, 3, 5}} {
		var encoded bytes.Buffer
		if err := writeResponse(&encoded, response{op: opMove, sequence: 1, timing: buckets}); err != nil || encoded.Len() != responseSize {
			t.Fatal("timing changed response size or failed serialization")
		}
		var data [responseSize]byte
		copy(data[:], encoded.Bytes())
		decoded, err := decodeResponse(data)
		if err != nil || decoded.timing != buckets || decoded.status != 0 {
			t.Fatal("timing changed successful reply semantics")
		}
		data[23] = 1
		if _, err := decodeResponse(data); !errors.Is(err, ErrProtocol) {
			t.Fatal("timing enabled reserved-byte acceptance")
		}
	}
	var encoded bytes.Buffer
	if err := writeResponse(&encoded, response{op: opMove, sequence: 1, timing: [3]byte{6}}); !errors.Is(err, ErrProtocol) || encoded.Len() != 0 {
		t.Fatal("unbounded diagnostic bucket sent")
	}
	_ = writeResponse(&encoded, response{op: opMove, sequence: 1, timing: [3]byte{1}})
	var data [responseSize]byte
	copy(data[:], encoded.Bytes())
	data[20] = 6
	if _, err := decodeResponse(data); !errors.Is(err, ErrProtocol) {
		t.Fatal("unbounded diagnostic bucket received")
	}
	data[20], data[4] = 1, 1
	if _, err := decodeResponse(data); !errors.Is(err, ErrProtocol) {
		t.Fatal("legacy reply accepted timing extension")
	}
}

func TestTimingMeasurementPreservesAuthorizationAndInputFailures(t *testing.T) {
	cause := errors.New("denied")
	timing := &workerTiming{}
	calls := 0
	execute := measureExecution(func() error { return cause }, testExecute, timing)
	if err := execute(context.Background(), func() error { calls++; return nil }); err != cause || calls != 0 || timing.buckets[0] == 0 || timing.buckets[1] != 0 || timing.buckets[2] != 0 {
		t.Fatal("measurement bypassed failed authorization or invented input timing")
	}
	execute = measureExecution(func() error { return nil }, testExecute, timing)
	if err := execute(context.Background(), func() error { calls++; return cause }); err != cause || calls != 1 || timing.buckets[1] == 0 || timing.buckets[2] == 0 {
		t.Fatal("measurement changed input failure or omitted elapsed stages")
	}
}

func TestTimingSamplerAggregatesMaximumAndFlushesOnce(t *testing.T) {
	var events []string
	sampler := timingSampler{emit: func(code string) { events = append(events, code) }}
	start := time.Unix(1, 0)
	sampler.observe([7]byte{1, 3}, start)
	for i := 0; i < 1000; i++ {
		sampler.observe([7]byte{1, 2, 255}, start.Add(time.Second))
	}
	if len(events) != 0 {
		t.Fatal("per-input events emitted instead of aggregate diagnostics")
	}
	sampler.observe([7]byte{2, 1}, start.Add(15*time.Second))
	if len(events) != 2 || events[0] != timingCode(0, 2) || events[1] != timingCode(1, 3) {
		t.Fatal("window lost maximum or accepted unbounded buckets")
	}
	sampler.flush(start.Add(16 * time.Second))
	if len(events) != 2 || timingCode(-1, 1) != "" || timingCode(0, 6) != "" || timingBucket(-1) != 0 {
		t.Fatal("empty window duplicated events or accepted invalid fields")
	}
	for i, duration := range []time.Duration{0, time.Millisecond, 10 * time.Millisecond, 100 * time.Millisecond, time.Second} {
		if timingBucket(duration) != byte(i+1) {
			t.Fatal("duration boundary classified incorrectly")
		}
	}
}

func TestTimingRoundtripPreservesInputAndRecovery(t *testing.T) {
	server, conn := net.Pipe()
	lease, timing, input := testLease(), &workerTiming{}, &recordingInput{}
	done := make(chan finish, 1)
	go func() {
		do := measureExecution(func() error { return nil }, testExecute, timing)
		report, err := serve(context.Background(), server, input, lease, do, testExecute, time.Now, timing)
		done <- finish{report, err}
	}()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), lease)
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	client.SetTimingObserver(func(code string) { events = append(events, code) })
	if err := client.Move(context.Background(), 1, 1); err != nil {
		t.Fatal("measured authenticated input failed")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if result := await(t, done); result.err != nil || result.report.Completed != 1 || result.report.Recovery.Remaining != 0 || len(events) != 7 {
		t.Fatal("timing altered input/report or omitted diagnostic stages")
	}
}

func TestForeignAckCannotSupplyWorkerTimings(t *testing.T) {
	server, conn := net.Pipe()
	client, err := NewClient(context.Background(), clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	var events []string
	client.SetTimingObserver(func(code string) { events = append(events, code) })
	go func() {
		defer server.Close()
		req, _ := readRequest(server)
		_ = writeResponse(server, response{op: req.op, sequence: req.sequence + 1, timing: [3]byte{5, 5, 5}})
	}()
	if err := client.Move(context.Background(), 1, 1); err == nil {
		t.Fatal("foreign reply accepted")
	}
	for _, event := range events {
		if strings.Contains(event, "worker_") {
			t.Fatal("unmatched acknowledgement supplied worker diagnostic data")
		}
	}
}
