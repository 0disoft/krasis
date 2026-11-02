package inputworker

import (
	"bytes"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

// Codec CPU/allocations only; excludes real pipes, Windows scheduling and input.
func BenchmarkInputTimingSuffixCodec(b *testing.B) {
	for _, detail := range []bool{false, true} {
		name := "base"
		ack := response{sequence: 1, op: opMove, measured: 4, millis: [3]uint16{0, 0, 1}}
		ack.timing[2] = timingBucket(1000000)
		if detail {
			name = "detailed"
			ack.applyDetail = &handoff.WorkerApplyProfile{Measured: 1, Micros: [2]uint32{1000, 0}, SubmissionCalls: 1, SubmissionMicros: 900, NativeIPC: &handoff.NativeIPCProfile{Micros: [6]uint32{5, 10, 2, 950, 15, 18}}}
			ack.applyDetail.NativeIPC.ParentMeasured = true
			ack.applyDetail.NativeIPC.ParentMicros = [3]uint32{2, 3, 4}
		}
		b.Run(name, func(b *testing.B) {
			var wire bytes.Buffer
			wire.Grow(maxResponseSize)
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				wire.Reset()
				if err := writeResponse(&wire, ack); err != nil {
					b.Fatal(err)
				}
				if _, err := readResponse(&wire); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
