package peer

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

func BenchmarkPointerBatchStorage(b *testing.B) {
	for _, count := range []int{1, application.MaxPointerMoveBatch} {
		b.Run(fmt.Sprint(count), func(b *testing.B) {
			r := &inputReadAhead{ctx: context.Background(), frames: make(chan queuedSessionFrame, application.MaxPointerMoveBatch)}
			frames := make([]transport.Frame, count)
			for i := range frames {
				payload, err := transport.EncodeInputPayload(control.InputEnvelope{
					SessionID: strings.Repeat("a", 32), Epoch: strings.Repeat("b", 32), Sequence: uint64(i + 1),
				}, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1})
				if err != nil {
					b.Fatal(err)
				}
				frames[i] = transport.Frame{Kind: transport.MessageInput, Payload: payload}
			}
			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				for _, frame := range frames[1:] {
					r.frames <- queuedSessionFrame{frame: frame}
				}
				envelopes, move := r.pointerBatch(frames[0])
				if len(envelopes) != count || move.DeltaX != int32(count) || envelopes[count-1].Sequence != uint64(count) {
					b.Fatal("pointer batch changed")
				}
			}
		})
	}
}
