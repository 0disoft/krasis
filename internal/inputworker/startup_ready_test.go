package inputworker

import (
	"bytes"
	"encoding/binary"
	"testing"
	"testing/iotest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestStartupReadyRoundtripAndLegacyPreserveFollowingFrame(t *testing.T) {
	var profile handoff.StartupProfile
	profile.Record(handoff.ServiceLaunch, 7*time.Millisecond)
	profile.Record(handoff.WorkerDesktopReady, 0)
	for _, legacy := range []bool{false, true} {
		var buffer bytes.Buffer
		if legacy {
			buffer.Write(readyFrame[:])
		} else if err := writeStartupReady(&buffer, profile); err != nil {
			t.Fatal(err)
		}
		buffer.WriteByte('S')
		got, err := readStartupReady(iotest.OneByteReader(&buffer))
		if err != nil || (legacy && got != nil) || (!legacy && (got == nil || *got != profile)) {
			t.Fatalf("readiness changed: legacy=%v profile=%+v err=%v", legacy, got, err)
		}
		if next, err := buffer.ReadByte(); err != nil || next != 'S' {
			t.Fatal("readiness consumed the following protocol frame")
		}
	}
}

func TestStartupReadyRejectsMalformedReceiptButDropsInvalidNumbers(t *testing.T) {
	var profile handoff.StartupProfile
	profile.Record(handoff.ServiceLaunch, time.Millisecond)
	var buffer bytes.Buffer
	if err := writeStartupReady(&buffer, profile); err != nil {
		t.Fatal(err)
	}
	packet := append([]byte(nil), buffer.Bytes()...)
	for _, length := range []int{0, 7, 8, startupReadySize - 1} {
		if _, err := readStartupReady(bytes.NewReader(packet[:length])); err != ErrProtocol {
			t.Fatalf("truncated receipt accepted: %d", length)
		}
	}
	for _, offset := range []int{0, 4, 6, 7} {
		bad := append([]byte(nil), packet...)
		bad[offset]++
		if _, err := readStartupReady(bytes.NewReader(bad)); err != ErrProtocol {
			t.Fatalf("malformed readiness accepted: %d", offset)
		}
	}
	for _, mutate := range []func([]byte){
		func(b []byte) { binary.BigEndian.PutUint32(b[8:12], uint32(handoff.Window.Microseconds()+1)) },
		func(b []byte) { binary.BigEndian.PutUint32(b[12:16], 1) },
		func(b []byte) { b[5] = 0 },
	} {
		bad := append([]byte(nil), packet...)
		mutate(bad)
		reader := bytes.NewReader(append(bad, 'S'))
		if got, err := readStartupReady(reader); err != nil || got != nil {
			t.Fatal("invalid diagnostic numbers changed readiness")
		}
		if next, err := reader.ReadByte(); err != nil || next != 'S' {
			t.Fatal("discarded numeric diagnostics misaligned the protocol")
		}
	}
}

func TestStartupReadyInvalidLocalProfileFallsBackAndChildCannotOverwriteService(t *testing.T) {
	var buffer bytes.Buffer
	if err := writeStartupReady(&buffer, handoff.StartupProfile{}); err != nil || !bytes.Equal(buffer.Bytes(), readyFrame[:]) {
		t.Fatal("missing local diagnostics broke readiness")
	}
	var service, worker handoff.StartupProfile
	service.Record(handoff.ServiceLaunch, time.Millisecond)
	worker.Record(handoff.ServiceLaunch, time.Second)
	worker.Record(handoff.WorkerIdentity, 2*time.Millisecond)
	mergeWorkerStartup(&service, &worker)
	if service.Micros[handoff.ServiceLaunch] != 1000 || service.Micros[handoff.WorkerIdentity] != 2000 {
		t.Fatal("child replaced service-local timings or lost its own span")
	}
}

func FuzzStartupReady(f *testing.F) {
	f.Add(readyFrame[:])
	var profile handoff.StartupProfile
	profile.Record(handoff.ServiceLaunch, time.Millisecond)
	var buffer bytes.Buffer
	_ = writeStartupReady(&buffer, profile)
	f.Add(buffer.Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		reader := bytes.NewReader(data)
		profile, err := readStartupReady(reader)
		if profile != nil && (err != nil || !profile.Valid() || profile.Measured == 0) {
			t.Fatal("unvalidated diagnostic escaped parser")
		}
		if len(data)-reader.Len() > startupReadySize {
			t.Fatal("unbounded readiness read")
		}
	})
}
