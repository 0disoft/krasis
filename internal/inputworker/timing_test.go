package inputworker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strings"
	"testing"
	"testing/iotest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestDetailedTimingReplyKeepsLegacyFramingAndPresence(t *testing.T) {
	precise := response{op: opMove, sequence: 1, timing: [3]byte{1, 4, 2}, millis: [3]uint16{0, 197, 4}, measured: 7}
	legacy := response{op: opMove, sequence: 2, timing: [3]byte{1, 3, 5}}
	var encoded bytes.Buffer
	if err := writeResponse(&encoded, precise); err != nil || encoded.Len() != responseSize {
		t.Fatal("detailed ACK not fixed-size", err)
	}
	if !bytes.Equal(encoded.Bytes()[20:24], []byte{0x00, 0x53, 0x18, 0x01}) {
		t.Fatal("wire slots or byte order changed")
	}
	if err := writeResponse(&encoded, legacy); err != nil || encoded.Len() != 2*responseSize {
		t.Fatal("legacy framing changed", err)
	}
	reader := iotest.OneByteReader(&encoded)
	for _, want := range []response{precise, legacy} {
		got, err := readResponse(reader)
		if err != nil || got != want {
			t.Fatalf("mixed ACK frames: %+v, %v", got, err)
		}
	}
	if encoded.Len() != 0 {
		t.Fatal("ACK left trailing bytes")
	}
}

func TestWorkerTimingDetailPrecisionBoundsAndAbsence(t *testing.T) {
	for _, elapsed := range []time.Duration{-1, 0, 999 * time.Microsecond, time.Millisecond, 9999 * time.Microsecond, 10 * time.Millisecond, 99999 * time.Microsecond, 100 * time.Millisecond, time.Second, 1022 * time.Millisecond, 2 * time.Second, time.Hour} {
		for stage := 0; stage < 3; stage++ {
			timing := &workerTiming{}
			timing.record(stage, elapsed)
			var encoded bytes.Buffer
			want := response{op: opMove, sequence: 1, timing: timing.buckets, millis: timing.millis, measured: timing.measured}
			if err := writeResponse(&encoded, want); err != nil {
				t.Fatal("valid measurement rejected", elapsed, err)
			}
			got, err := readResponse(&encoded)
			if err != nil || got != want || got.measured != 1<<stage || got.millis[stage] > handoff.MaxWorkerMillis {
				t.Fatal("precision, saturation or absence lost", elapsed, got, err)
			}
		}
	}
}

func TestDetailedTimingRejectsInvalidDetailAndTruncatedFrames(t *testing.T) {
	ack := response{op: opMove, sequence: 1, timing: [3]byte{1, 4, 2}, millis: [3]uint16{0, 197, 4}, measured: 7}
	var encoded bytes.Buffer
	if err := writeResponse(&encoded, ack); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { b[20] |= 0x40 },                           // reserved bit30
		func(b []byte) { b[20] |= 0x80 },                           // reserved bit31
		func(b []byte) { binary.BigEndian.PutUint32(b[20:24], 0) }, // empty detail
	} {
		data := append([]byte(nil), encoded.Bytes()...)
		mutate(data)
		if _, err := readResponse(bytes.NewReader(data)); !errors.Is(err, ErrProtocol) {
			t.Fatal("malformed diagnostic frame accepted", err)
		}
	}
	for _, size := range []int{0, responseSize - 1} {
		if _, err := readResponse(io.LimitReader(bytes.NewReader(encoded.Bytes()), int64(size))); !errors.Is(err, ErrProtocol) {
			t.Fatal("truncated ACK accepted", size)
		}
	}
	for _, invalid := range []response{
		{op: opMove, sequence: 1, timing: [3]byte{1}, measured: 8},
		{op: opMove, sequence: 1, timing: [3]byte{1}, millis: [3]uint16{4}, measured: 1},
		{op: opMove, sequence: 1, timing: [3]byte{5}, millis: [3]uint16{1023}, measured: 1},
	} {
		encoded.Reset()
		if err := writeResponse(&encoded, invalid); !errors.Is(err, ErrProtocol) || encoded.Len() != 0 {
			t.Fatal("invalid detail serialized", err)
		}
	}
}

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
	if err := execute(context.Background(), func() error { calls++; return nil }); err != cause || calls != 0 || timing.buckets[0] == 0 || timing.buckets[1] != 0 || timing.buckets[2] != 0 || timing.measured != 1 {
		t.Fatal("measurement bypassed failed authorization or invented input timing")
	}
	execute = measureExecution(func() error { return nil }, testExecute, timing)
	if err := execute(context.Background(), func() error { calls++; return cause }); err != cause || calls != 1 || timing.buckets[1] == 0 || timing.buckets[2] == 0 || timing.measured != 7 {
		t.Fatal("measurement changed input failure or omitted elapsed stages")
	}
	execute = measureExecution(func() error { return nil }, func(context.Context, func() error) error { return cause }, timing)
	if err := execute(context.Background(), func() error { calls++; return nil }); err != cause || calls != 1 || timing.measured != 3 || timing.buckets[2] != 0 || timing.millis[2] != 0 {
		t.Fatal("desktop failure inherited prior apply measurement")
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
	var profile handoff.Summary
	ctx := handoff.WithSession(context.Background(), handoff.New(func(s handoff.Summary) { profile = s }))
	profileCtx := handoff.Begin(ctx, "receiver", "session", "epoch", time.Now())
	handoff.Backend(profileCtx, "installed")
	inputCtx, complete := handoff.Input(ctx, "receiver")
	if err := client.Move(inputCtx, 1, 1); err != nil {
		t.Fatal("measured authenticated input failed")
	}
	complete(nil)
	handoff.Finish(profileCtx, "ok")
	if !profile.Valid() || profile.First.WorkerMeasured != 7 || profile.First.Worker != timing.buckets || profile.First.WorkerMillis != timing.millis {
		t.Fatal("matching ACK detail not attached to original dispatch", profile)
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
	var profile handoff.Summary
	ctx := handoff.WithSession(context.Background(), handoff.New(func(s handoff.Summary) { profile = s }))
	profileCtx := handoff.Begin(ctx, "receiver", "session", "epoch", time.Now())
	handoff.Backend(profileCtx, "installed")
	inputCtx, complete := handoff.Input(ctx, "receiver")
	go func() {
		defer server.Close()
		req, _ := readRequest(server)
		_ = writeResponse(server, response{op: req.op, sequence: req.sequence + 1, timing: [3]byte{5, 5, 5}, millis: [3]uint16{1022, 1022, 1022}, measured: 7})
	}()
	err = client.Move(inputCtx, 1, 1)
	if err == nil {
		t.Fatal("foreign reply accepted")
	}
	complete(err)
	handoff.Finish(profileCtx, "error")
	if !profile.Valid() || profile.First.WorkerMeasured != 0 || profile.First.Worker != ([3]byte{}) {
		t.Fatal("foreign ACK polluted handoff profile")
	}
	for _, event := range events {
		if strings.Contains(event, "worker_") {
			t.Fatal("unmatched acknowledgement supplied worker diagnostic data")
		}
	}
}
