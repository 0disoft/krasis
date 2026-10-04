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
	profile.Record(handoff.WorkerInstalledValidation, time.Millisecond)
	profile.RecordLoad(time.Microsecond)
	buffer.Reset()
	_ = writeStartupReady(&buffer, profile)
	f.Add(buffer.Bytes())
	f.Fuzz(func(t *testing.T, data []byte) {
		reader := bytes.NewReader(data)
		profile, err := readStartupReady(reader)
		if profile != nil && (err != nil || !profile.Valid() || profile.Measured == 0) {
			t.Fatal("unvalidated diagnostic escaped parser")
		}
		if len(data)-reader.Len() > startupReadyLoadSize {
			t.Fatal("unbounded readiness read")
		}
	})
}

func TestStartupLoadReadyRoundtripBoundsAndFollowingFrame(t *testing.T) {
	var p handoff.StartupProfile
	p.Record(handoff.WorkerInstalledValidation, 20*time.Millisecond)
	p.RecordLoad(15 * time.Millisecond)
	var b bytes.Buffer
	if writeStartupReady(&b, p) != nil || b.Len() != startupReadyLoadSize || string(b.Bytes()[:4]) != "KRD3" {
		t.Fatal("load receipt missing")
	}
	packet := append([]byte(nil), b.Bytes()...)
	reader := bytes.NewReader(append(append([]byte(nil), packet...), 'S'))
	got, err := readStartupReady(iotest.OneByteReader(reader))
	if err != nil || got == nil || *got != p {
		t.Fatal("load detail lost", got, err)
	}
	if next, err := reader.ReadByte(); err != nil || next != 'S' {
		t.Fatal("extended receipt consumed next frame")
	}
	for length := startupReadySize; length < startupReadyLoadSize; length++ {
		if _, err := readStartupReady(bytes.NewReader(packet[:length])); err != ErrProtocol {
			t.Fatal("truncated load receipt accepted", length)
		}
	}
	for _, offset := range []int{startupReadySize + 4, startupReadySize + 5, startupReadySize + 6, startupReadySize + 7} {
		bad := append([]byte(nil), packet...)
		bad[offset] = 2
		if _, err := readStartupReady(bytes.NewReader(bad)); err != ErrProtocol {
			t.Fatal("malformed extension accepted")
		}
	}
	bad := append([]byte(nil), packet...)
	binary.BigEndian.PutUint32(bad[startupReadySize:startupReadySize+4], 21000)
	reader = bytes.NewReader(append(bad, 'S'))
	if got, err := readStartupReady(reader); err != nil || got != nil {
		t.Fatal("invalid numeric detail changed readiness")
	}
	if next, err := reader.ReadByte(); err != nil || next != 'S' {
		t.Fatal("discarded load detail consumed next frame")
	}
	var service handoff.StartupProfile
	service.Record(handoff.ServiceLaunch, time.Millisecond)
	mergeWorkerStartup(&service, &p)
	if !service.Valid() || !service.LoadMeasured || service.LoadMicros != 15000 || service.Micros[handoff.ServiceLaunch] != 1000 {
		t.Fatal("merged load detail lost or replaced service measurement")
	}
}
