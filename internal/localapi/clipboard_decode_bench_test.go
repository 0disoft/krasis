package localapi

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func BenchmarkDecodeClipboardRequest8MiB(b *testing.B) {
	data, err := json.Marshal(ClipboardRequest{Action: "SYNC", Peer: strings.Repeat("a", 64),
		Format: "files", Data: make([]byte, 8<<20)})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		var value ClipboardRequest
		if err := decodeClipboardRequest(bytes.NewReader(data), &value); err != nil {
			b.Fatal(err)
		}
		if len(value.Data) != 8<<20 {
			b.Fatal("decoded payload changed")
		}
	}
}
