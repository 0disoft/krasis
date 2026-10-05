package transport

import (
	"bytes"
	"testing"
	"time"
)

func TestReadDetailObserverPreservesRuntimeProgressAndFragmentedFrame(t *testing.T) {
	var wire bytes.Buffer
	writer, _ := NewWriter(&wire)
	want := Frame{Kind: MessageInput, Payload: []byte("private-test-payload")}
	if err := writer.WriteFrame(want); err != nil {
		t.Fatal(err)
	}
	reader, _ := NewReader(oneByteReader{source: bytes.NewReader(wire.Bytes())})
	var runtimeBytes, detailBytes int
	var runtimeAt time.Time
	reader.SetReadProgressObserver(func(n int, at time.Time) { runtimeBytes += n; runtimeAt = at })
	if !reader.SetReadDetailObserver(func(n int, at time.Time) {
		detailBytes += n
		if at != runtimeAt || at.IsZero() {
			t.Fatal("callbacks used different or absent read observations")
		}
	}) {
		t.Fatal("detail observer unavailable")
	}
	got, err := reader.ReadFrame()
	if err != nil || got.Kind != want.Kind || !bytes.Equal(got.Payload, want.Payload) || runtimeBytes != wire.Len() || detailBytes != runtimeBytes {
		t.Fatal("detail callback changed framing or replaced runtime progress", err)
	}
	reader.SetReadDetailObserver(nil)
	if _, err := reader.ReadFrame(); err == nil {
		t.Fatal("EOF fabricated a frame")
	}
	if (*Reader)(nil).SetReadDetailObserver(nil) || (*PeerConnection)(nil).SetReadDetailObserver(nil) || (&PeerConnection{}).SetReadDetailObserver(nil) {
		t.Fatal("missing reader fabricated byte-observer availability")
	}
}
