//go:build windows

package inputworker

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestNativeSourceSubmissionPreservesFrameAndUnsubmittedProof(t *testing.T) {
	server, client := nativePair(t)
	deadline := time.Now().Add(2 * time.Second)
	if err := server.SetReadDeadline(deadline); err != nil {
		t.Fatal(err)
	}
	packet := sourcePacket{kind: sourceCommand, op: sourceActivate, sequence: 1, binding: Binding{1}}
	wire, err := encodeSourcePacket(packet)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		packet sourcePacket
		tick   uint64
		err    error
	}
	done := make(chan result, 1)
	go func() { p, e := readSourcePacket(server); done <- result{p, workerIPCTick(), e} }()
	var submitted uint64
	sent := workerIPCTick()
	if err := writeSourceCommand(client, wire[:], deadline, &submitted); err != nil {
		t.Fatal(err)
	}
	returned := workerIPCTick()
	got := <-done
	if got.err != nil || got.packet != packet || submitted < sent || submitted == 0 || submitted > returned || submitted > got.tick {
		t.Fatal(submitted, returned, got)
	}
	// Admission expires before any WriteFile. No frame may be replayed or
	// mistaken for a successful timing observation.
	submitted = 999
	err = writeSourceCommand(client, wire[:], time.Now().Add(-time.Second), &submitted)
	var failure *sourceIOFailure
	if !errors.As(err, &failure) || !failure.unsubmitted || !errors.Is(err, os.ErrDeadlineExceeded) || submitted != 0 {
		t.Fatal(submitted, err)
	}
	if err := server.SetReadDeadline(time.Now().Add(20 * time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	var data [1]byte
	if n, e := server.Read(data[:]); n != 0 || !errors.Is(e, os.ErrDeadlineExceeded) {
		t.Fatal("unsubmitted frame reached worker", n, e)
	}
}
