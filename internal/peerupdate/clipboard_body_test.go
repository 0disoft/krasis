package peerupdate

import (
	"bytes"
	"errors"
	"io"
	"testing"
)

var clipboardBodyBenchmarkSink []byte

func BenchmarkClipboardBody(b *testing.B) {
	for _, size := range []int{64 << 10, 4 << 20, MaxClipboardFileBytes} {
		name := map[int]string{64 << 10: "64KiB", 4 << 20: "4MiB", MaxClipboardFileBytes: "64MiB"}[size]
		b.Run(name, func(b *testing.B) {
			data := make([]byte, size)
			reader := bytes.NewReader(data)
			b.SetBytes(int64(size))
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				reader.Reset(data)
				body, err := readClipboardBody(reader, size)
				if err != nil || len(body) != size {
					b.Fatal(len(body), err)
				}
				clipboardBodyBenchmarkSink = body
			}
		})
	}
	clipboardBodyBenchmarkSink = nil
}

// Transport framing errors must survive even if all advertised bytes arrived.
type clipboardBodyErrorReader struct {
	remaining int
	failure   error
	chunk     int
}

func (r *clipboardBodyErrorReader) Read(p []byte) (int, error) {
	n := min(len(p), r.remaining)
	if r.chunk > 0 {
		n = min(n, r.chunk)
	}
	clear(p[:n])
	r.remaining -= n
	if r.remaining == 0 {
		return n, r.failure
	}
	return n, nil
}

func TestClipboardBodyMatchesBoundedReadAll(t *testing.T) {
	for _, size := range []int{0, 1, 511, 512, 513, 1024, 4097} {
		for _, limit := range []int{0, 1, 512, 1024, 4097} {
			for _, chunk := range []int{0, 1, 7} {
				for _, failure := range []error{io.EOF, io.ErrUnexpectedEOF, errors.New("transport failed")} {
					want, wantErr := io.ReadAll(io.LimitReader(&clipboardBodyErrorReader{size, failure, chunk}, int64(limit)+1))
					got, gotErr := readClipboardBody(&clipboardBodyErrorReader{size, failure, chunk}, limit)
					if !bytes.Equal(got, want) || !errors.Is(gotErr, wantErr) {
						t.Fatalf("size=%d limit=%d: got %d/%v want %d/%v", size, limit, len(got), gotErr, len(want), wantErr)
					}
				}
			}
		}
	}
}
