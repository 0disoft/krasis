package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"reflect"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
)

type ackReadStep struct {
	limit int
	delay time.Duration
	err   error
}
type scriptedACKReader struct {
	data  []byte
	steps []ackReadStep
	sizes []int
}

func (r *scriptedACKReader) Read(data []byte) (int, error) {
	r.sizes = append(r.sizes, len(data))
	if len(r.steps) == 0 {
		if len(r.data) == 0 {
			return 0, io.EOF
		}
		n := copy(data, r.data)
		r.data = r.data[n:]
		return n, nil
	}
	step := r.steps[0]
	r.steps = r.steps[1:]
	time.Sleep(step.delay)
	n := copy(data[:min(len(data), step.limit)], r.data)
	r.data = r.data[n:]
	return n, step.err
}

func TestACKReadSpansPreserveReadsFramingAndPartialFailures(t *testing.T) {
	var wire bytes.Buffer
	if err := writeResponse(&wire, response{op: opCursorEntry, sequence: 1, beforeX: -3, beforeY: 4, targetX: 5, targetY: 6, x: 5, y: 6}); err != nil {
		t.Fatal(err)
	}
	entry := append([]byte(nil), wire.Bytes()...)
	for _, test := range []struct {
		name               string
		steps              []ackReadStep
		badHeader          bool
		want               [3]uint32
		progress, measured uint8
		wantErr            error
	}{
		{"first_return", []ackReadStep{{24, 80 * time.Millisecond, nil}}, false, [3]uint32{80000, 0, 0}, handoff.ACKReadComplete, 7, nil},
		{"header_and_suffix", []ackReadStep{{1, 10 * time.Millisecond, nil}, {23, 30 * time.Millisecond, nil}, {24, 40 * time.Millisecond, nil}}, false, [3]uint32{10000, 30000, 40000}, handoff.ACKReadComplete, 7, nil},
		{"no_data", []ackReadStep{{0, 20 * time.Millisecond, io.EOF}}, false, [3]uint32{20000, 0, 0}, handoff.ACKReadNoData, 1, io.EOF},
		{"partial_header", []ackReadStep{{3, 10 * time.Millisecond, nil}, {0, 20 * time.Millisecond, io.EOF}}, false, [3]uint32{10000, 20000, 0}, handoff.ACKReadData, 3, io.ErrUnexpectedEOF},
		{"bad_header", []ackReadStep{{24, 10 * time.Millisecond, nil}}, true, [3]uint32{10000, 0, 0}, handoff.ACKReadData, 3, ErrProtocol},
		{"partial_suffix", []ackReadStep{{24, 10 * time.Millisecond, nil}, {3, 30 * time.Millisecond, nil}, {0, 50 * time.Millisecond, io.EOF}}, false, [3]uint32{10000, 0, 80000}, handoff.ACKReadHeader, 7, io.ErrUnexpectedEOF},
		{"suffix_timeout", []ackReadStep{{24, 10 * time.Millisecond, nil}, {0, 50 * time.Millisecond, os.ErrDeadlineExceeded}}, false, [3]uint32{10000, 0, 50000}, handoff.ACKReadHeader, 7, os.ErrDeadlineExceeded},
	} {
		t.Run(test.name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				data := append([]byte(nil), entry...)
				if test.badHeader {
					data[0] = 'X'
				}
				plain := &scriptedACKReader{data: data, steps: append([]ackReadStep(nil), test.steps...)}
				observed := &scriptedACKReader{data: data, steps: append([]ackReadStep(nil), test.steps...)}
				want, plainErr := readResponse(plain)
				var p handoff.ACKReadProfile
				got, err := readResponseObserved(observed, &p)
				if !reflect.DeepEqual(want, got) || !reflect.DeepEqual(plain.sizes, observed.sizes) || len(plain.data) != len(observed.data) || (plainErr == nil) != (err == nil) || test.wantErr != nil && !errors.Is(err, test.wantErr) {
					t.Fatal("observation changed reader framing or outcome", plainErr, err)
				}
				if !p.Valid() || p.Micros != test.want || p.Progress != test.progress || p.Measured != test.measured {
					t.Fatalf("wrong ACK spans: %+v", p)
				}
				if err != nil && got != (response{}) {
					t.Fatal("partial response exposed cursor state")
				}
			})
		})
	}
}

func TestACKReadPositiveReturnWithEOFAndNextACKAlignment(t *testing.T) {
	for _, op := range []operation{opMove, opCursorEntry, opCursorEntryProbe} {
		var wire bytes.Buffer
		_ = writeResponse(&wire, response{op: op, sequence: 1, beforeX: -3, x: 7, targetX: 8})
		if op == opMove {
			r := &scriptedACKReader{data: wire.Bytes(), steps: []ackReadStep{{24, 0, io.EOF}}}
			var p handoff.ACKReadProfile
			if _, err := readResponseObserved(r, &p); err != nil || !p.Valid() || p.Progress != handoff.ACKReadComplete || len(r.sizes) != 1 {
				t.Fatal("positive read with EOF changed completion", err, p)
			}
			continue
		}
		_ = writeResponse(&wire, response{op: opMove, sequence: 2})
		r := &scriptedACKReader{data: wire.Bytes()}
		var p handoff.ACKReadProfile
		if got, err := readResponseObserved(r, &p); err != nil || got.beforeX != -3 || !p.Valid() || p.Progress != handoff.ACKReadComplete {
			t.Fatal("entry lost suffix", err)
		}
		if next, err := readResponse(r); err != nil || next.sequence != 2 || !reflect.DeepEqual(r.sizes, []int{24, len(wire.Bytes()) - 48, 24}) {
			t.Fatal("next ACK misaligned or probe read added", r.sizes, err)
		}
	}
}

func TestACKReadAttachesToSameEntryOrInputEvenWhenACKIsLostOrForeign(t *testing.T) {
	for _, sample := range []string{"entry", "input"} {
		for _, mode := range []string{"ok", "rejected", "foreign", "partial_suffix", "no_data"} {
			t.Run(sample+"/"+mode, func(t *testing.T) {
				var out []handoff.Summary
				collector := handoff.New(func(s handoff.Summary) { out = append(out, s) })
				ctx := handoff.WithSession(context.Background(), collector)
				p := handoff.Begin(ctx, "receiver", "session", "epoch", time.Now())
				handoff.Backend(p, "installed")
				x, complete := handoff.Entry(p)
				if sample == "input" {
					x, complete = handoff.Input(ctx, "receiver")
				}
				server, pipe := net.Pipe()
				client, err := NewClient(ctx, clientChannel(pipe, authorityService), testLease())
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				done := make(chan struct{})
				go func() {
					defer close(done)
					defer server.Close()
					command, err := readRequest(server)
					if err != nil {
						return
					}
					ack := response{op: command.op, sequence: command.sequence, timing: [3]byte{1, 2, 2}, millis: [3]uint16{0, 1, 2}, measured: 7, applyDetail: &handoff.WorkerApplyProfile{Micros: [2]uint32{2000, 0}, Measured: 1}}
					if mode == "foreign" {
						ack.sequence++
					}
					if mode == "rejected" {
						ack.status, ack.certainty = 1, 1
					}
					var w bytes.Buffer
					_ = writeResponse(&w, ack)
					if mode == "no_data" {
						return
					}
					if mode == "partial_suffix" {
						_, _ = server.Write(w.Bytes()[:w.Len()-1])
						return
					}
					_, _ = server.Write(w.Bytes())
				}()
				_, err = client.EnterCursor(x, 1, 2)
				complete(err)
				_ = client.Close()
				<-done
				handoff.Finish(p, "ok")
				if len(out) != 1 || !out[0].Valid() {
					t.Fatal("invalid handoff", out)
				}
				v := out[0].Entry
				if sample == "input" {
					v = &out[0].First
				}
				if v == nil || v.ACKRead == nil {
					t.Fatal("read profile missing from matching sample")
				}
				want := handoff.ACKReadMatched
				if mode == "foreign" {
					want = handoff.ACKReadComplete
				}
				if mode == "partial_suffix" {
					want = handoff.ACKReadHeader
				}
				if mode == "no_data" {
					want = handoff.ACKReadNoData
				}
				if v.ACKRead.Progress != want {
					t.Fatal("false matching or completion", v.ACKRead)
				}
				matched := mode == "ok" || mode == "rejected"
				if (v.WorkerMeasured != 0) != matched || (v.WorkerApply != nil) != matched {
					t.Fatal("worker details accepted without complete matching ACK")
				}
				if !matched {
					if c, ok := application.InjectionCertaintyOf(err); !ok || c != application.InjectionOutcomeUnknown {
						t.Fatal("unknown ACK became success", err)
					}
				}
				if client.sequence != 2 {
					t.Fatal("ACK observation retransmitted input")
				}
				if handoff.WantWorkerApply(x) {
					t.Fatal("read profile consumed another probe")
				}
			})
		}
	}
}
