package inputworker

import (
	"bytes"
	"context"
	"io"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestServiceIPCKeepsInnerProfileAndNextACKAligned(t *testing.T) {
	for _, inner := range []*handoff.NativeIPCProfile{nil,
		{Micros: [6]uint32{1, 2, 3, 4, 5, 6}},
		{Micros: [6]uint32{1, 2, 3, 4, 5, 6}, ParentMeasured: true, ParentMicros: [3]uint32{7, 8, 9}},
	} {
		var wire bytes.Buffer
		ack := response{sequence: 1, op: opMove, applyDetail: &handoff.WorkerApplyProfile{Measured: 1, Micros: [2]uint32{100, 0}, NativeIPC: inner},
			ipc: &nativeIPCWire{profile: inner, outer: [3]uint64{100, 110, 0}}}
		if err := writeResponse(&wire, ack); err != nil {
			t.Fatal(err)
		}
		if err := writeResponse(&wire, response{sequence: 2, op: opMove}); err != nil {
			t.Fatal(err)
		}
		r := newACKReader(&wire)
		got, err := readResponse(r)
		if err != nil || got.ipc == nil || got.ipc.outer[0] != 100 || got.ipc.outer[1] == 0 || got.applyDetail == nil {
			t.Fatal("outer exchange displaced inner detail", got, err)
		}
		if inner != nil && (got.applyDetail.NativeIPC == nil || *got.applyDetail.NativeIPC != *inner) {
			t.Fatal("inner timing changed")
		}
		if next, err := readResponse(r); err != nil || next.sequence != 2 {
			t.Fatal("outer suffix consumed next ACK", next, err)
		}
	}
	data := make([]byte, nativeIPCSize)
	(&nativeIPCWire{outer: [3]uint64{100, 110, 120}}).encode(data)
	for _, offset := range []int{1, 2, 7, 8, 32} {
		bad := bytes.Clone(data)
		bad[offset] = 0xff
		if decodeNativeIPC(bad) != nil {
			t.Fatal("outer reserved/absent inner bytes accepted", offset)
		}
	}
}

// A real QPC test: delays before reply submission and after positive Read must
// remain distinguishable, without an extra input, read or round trip.
func TestServiceIPCSeparatesHandlerFromAppReadResumption(t *testing.T) {
	if workerIPCTick() == 0 {
		t.Skip("same-host QPC unavailable")
	}
	var summaries []handoff.Summary
	collector := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	ctx := handoff.WithSession(context.Background(), collector)
	p := handoff.Begin(ctx, "receiver", "s", "e", time.Now())
	handoff.Backend(p, "installed")
	server, conn := net.Pipe()
	defer server.Close()
	stream := &serviceReadDelay{Conn: conn}
	client, err := NewClient(ctx, clientChannel(stream, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	result := make(chan error, 1)
	go func() {
		command, err := readRequest(server)
		if err != nil {
			result <- err
			return
		}
		at := workerIPCTick()
		time.Sleep(12 * time.Millisecond)
		result <- writeResponse(server, response{sequence: command.sequence, op: command.op,
			applyDetail: &handoff.WorkerApplyProfile{Measured: 1}, ipc: &nativeIPCWire{outer: [3]uint64{at, at, 0}}})
	}()
	i, done := handoff.Input(p, "receiver")
	if err := client.Move(i, 1, 0); err != nil {
		t.Fatal(err)
	}
	done(nil)
	if err := <-result; err != nil {
		t.Fatal(err)
	}
	handoff.Finish(p, "ok")
	if len(summaries) != 1 || summaries[0].First.ACKRead == nil {
		t.Fatal("matching app read profile missing")
	}
	service := summaries[0].First.ACKRead.ServiceIPC
	if service == nil || service.Micros[2] < 12000 || service.Micros[4] < 18000 || service.Micros[0] != 0 || summaries[0].First.ACKRead.ServiceNativeWrite {
		t.Fatal("handler and app resumption mixed", service)
	}
}

type serviceReadDelay struct{ net.Conn }

func (r *serviceReadDelay) Read(data []byte) (int, error) {
	n, err := r.Conn.Read(data)
	if n > 0 {
		time.Sleep(18 * time.Millisecond)
	}
	return n, err
}

var _ io.Reader = (*serviceReadDelay)(nil)

func TestServiceIPCProductionWorkerPopulatesOuterWithoutInnerProfile(t *testing.T) {
	if workerIPCTick() == 0 {
		t.Skip("same-host QPC unavailable")
	}
	var summaries []handoff.Summary
	c := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	ctx := handoff.WithSession(context.Background(), c)
	p := handoff.Begin(ctx, "receiver", "s", "e", time.Now())
	handoff.Backend(p, "installed")
	server, conn := net.Pipe()
	defer server.Close()
	client, err := NewClient(ctx, clientChannel(conn, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	finished := make(chan error, 1)
	go func() {
		_, err := serve(ctx, server, &recordingInput{}, testLease(), func(ctx context.Context, work func() error) error { return work() }, nil, time.Now)
		finished <- err
	}()
	i, done := handoff.Input(ctx, "receiver")
	if err := client.Move(i, 1, 0); err != nil {
		t.Fatal(err)
	}
	done(nil)
	handoff.Finish(p, "ok")
	if len(summaries) != 1 || summaries[0].First.ACKRead == nil || summaries[0].First.ACKRead.ServiceIPC == nil {
		t.Fatal("production service omitted requested outer exchange")
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
	if err := <-finished; err != nil {
		t.Fatal(err)
	}
}
