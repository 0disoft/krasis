package peerupdate

import (
	"strings"
	"testing"
	"time"
)

var clipboardSnapshotBenchmarkSink any

func BenchmarkClipboardOutgoing64MiB(b *testing.B) {
	peer, id := strings.Repeat("a", 64), strings.Repeat("b", 32)
	m := &Manager{clipboard: clipboardState{peer: peer, out: &ClipboardValue{
		ID: id, Format: "files", Data: make([]byte, MaxClipboardFileBytes), expires: time.Now().Add(time.Hour),
	}}}
	b.ReportAllocs()
	for b.Loop() {
		clipboardSnapshotBenchmarkSink = m.clipboardOutgoing(peer, id)
	}
	clipboardSnapshotBenchmarkSink = nil
}
