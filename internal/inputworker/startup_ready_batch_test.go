package inputworker

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"testing/iotest"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func startupBatchPacket(t testing.TB) []byte {
	t.Helper()
	p := handoff.StartupProfile{Prepared: true, PreparationDecision: handoff.PreparationUsed}
	p.Record(handoff.ServiceLaunch, time.Millisecond)
	p.Record(handoff.ServiceChildConnect, time.Millisecond)
	p.Record(handoff.WorkerInstalledValidation, time.Millisecond)
	p.RecordLoad(time.Microsecond)
	p.RecordPreparationWait(time.Microsecond)
	p.ServiceReadiness.Record(handoff.ServiceLoginAdmission, time.Microsecond)
	p.Native[0] = handoff.NativeStartupProfile{Measured: 31, Micros: [5]uint32{1}}
	p.ImageHash = handoff.ImageHashProfile{Backend: 1, Micros: [2]uint32{0, 1}}
	var wire bytes.Buffer
	if err := writeStartupReady(&wire, p); err != nil {
		t.Fatal(err)
	}
	if wire.Len() != startupReadyImageHashSize {
		t.Fatal("expected latest readiness envelope")
	}
	return wire.Bytes()
}

type startupCountingReader struct {
	io.Reader
	calls int
}

func (r *startupCountingReader) Read(p []byte) (int, error) {
	r.calls++
	return r.Reader.Read(p)
}

func TestStartupReadyBatchesAvailableBodyWithoutReadingNextFrame(t *testing.T) {
	packet := startupBatchPacket(t)
	for _, lease := range []bool{false, true} {
		reader := bytes.NewReader(append(append([]byte(nil), packet...), 'S'))
		counted := &startupCountingReader{Reader: reader}
		read := readStartupReady
		if lease {
			read = readInputLeaseReady
		}
		if p, err := read(counted); err != nil || p == nil {
			t.Fatal(p, err)
		}
		if counted.calls != 2 {
			t.Fatalf("lease=%v: readiness Read calls=%d, want 2", lease, counted.calls)
		}
		if next, err := reader.ReadByte(); err != nil || next != 'S' {
			t.Fatal("readiness consumed the following frame")
		}
	}
}

func TestStartupReadyBatchAllVersionsAndTruncation(t *testing.T) {
	latest := startupBatchPacket(t)
	sizes := []int{8, startupReadySize, startupReadyLoadSize, startupReadyPreparedSize,
		startupReadyDecisionSize, startupReadyWaitSize, startupReadyServiceSize,
		startupReadyNativeSize, startupReadyImageHashSize}
	for version, size := range sizes {
		packet := append([]byte(nil), latest[:size]...)
		packet[3], packet[4] = byte('1'+version), byte(1+version)
		if version == 0 {
			packet = append([]byte(nil), readyFrame[:]...)
		}
		reader := bytes.NewReader(append(packet, 'S'))
		p, err := readStartupReady(iotest.OneByteReader(reader))
		if err != nil || version > 0 && p == nil || version == 0 && p != nil {
			t.Fatalf("version=%d: profile=%v err=%v", version+1, p, err)
		}
		if next, err := reader.ReadByte(); err != nil || next != 'S' {
			t.Fatalf("version=%d consumed the next frame", version+1)
		}
		for n := 0; n < size; n++ {
			want := io.ErrUnexpectedEOF
			if n == 0 {
				want = io.EOF
			}
			for _, boundary := range sizes {
				if n == boundary {
					want = io.EOF
				}
			}
			if _, err := readStartupReady(bytes.NewReader(packet[:n])); !errors.Is(err, ErrProtocol) || !errors.Is(err, want) {
				t.Fatalf("version=%d accepted truncated length=%d: %v", version+1, n, err)
			}
		}
	}
}

func TestStartupReadyBatchPreservesReadErrorCause(t *testing.T) {
	cause := errors.New("read canceled")
	packet := startupBatchPacket(t)
	reader := io.MultiReader(bytes.NewReader(packet[:startupReadyNativeSize+1]), iotest.ErrReader(cause))
	if _, err := readStartupReady(reader); !errors.Is(err, ErrProtocol) || !errors.Is(err, cause) {
		t.Fatal("lost protocol classification or read error", err)
	}
}

func BenchmarkStartupReadyBatch(b *testing.B) {
	packet := startupBatchPacket(b)
	b.ReportAllocs()
	for b.Loop() {
		reader := &startupCountingReader{Reader: bytes.NewReader(packet)}
		if _, err := readInputLeaseReady(reader); err != nil {
			b.Fatal(err)
		}
		b.ReportMetric(float64(reader.calls), "reads/op")
	}
}
