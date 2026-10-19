//go:build windows

package inputworker

import (
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

type sourceReadCounter struct {
	io.Reader
	calls int
	want  int
}

func (r *sourceReadCounter) Read(p []byte) (int, error) {
	r.calls++
	r.want = max(r.want, len(p))
	return r.Reader.Read(p)
}

func sourceStreamBatch(t testing.TB, count int) []byte {
	t.Helper()
	var wire []byte
	for i := range count {
		p := sourcePacket{kind: sourceBoundary, sequence: uint64(i + 1), binding: Binding{1},
			action: control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, x: int32(i)}
		data, err := encodeSourcePacket(p)
		if err != nil {
			t.Fatal(err)
		}
		wire = append(wire, data[:]...)
	}
	return wire
}

func TestSourcePacketReaderDrainsOneNativeBatchInOrder(t *testing.T) {
	server, client := nativePair(t)
	wire := sourceStreamBatch(t, sourceBoundaryBatchLimit)
	if _, err := server.Write(wire); err != nil {
		t.Fatal(err)
	}
	counted := &sourceReadCounter{Reader: client}
	reader := newSourcePacketReader(counted)
	for i := range sourceBoundaryBatchLimit {
		p, err := readSourcePacket(reader)
		if err != nil || p.sequence != uint64(i+1) || p.x != int32(i) || p.binding != (Binding{1}) {
			t.Fatalf("packet %d changed: %+v, %v", i, p, err)
		}
	}
	if counted.calls != 1 || counted.want != sourceReadBufferSize || reader.Buffered() != 0 {
		t.Fatalf("batch read not bounded/amortized: %+v, buffered=%d", counted, reader.Buffered())
	}
}

func TestSourcePacketReaderReturnsSingleNativeACKWithoutFillingBuffer(t *testing.T) {
	server, client := nativePair(t)
	if err := client.SetReadDeadline(time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
	ack := sourcePacket{kind: sourceReply, op: sourceActivate, sequence: 1, reply: 7, binding: Binding{1}}
	wire, err := encodeSourcePacket(ack)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := server.Write(wire[:]); err != nil {
		t.Fatal(err)
	}
	got, err := readSourcePacket(newSourcePacketReader(client))
	if err != nil || got != ack {
		t.Fatalf("single ACK delayed or changed: %+v, %v", got, err)
	}
}

func TestSourcePacketReaderPreservesNativeCloseAndTruncation(t *testing.T) {
	for _, truncated := range []bool{false, true} {
		t.Run(fmt.Sprint(truncated), func(t *testing.T) {
			server, client := nativePair(t)
			if truncated {
				if _, err := server.Write([]byte{'K', 'S', 'C', '1'}); err != nil {
					t.Fatal(err)
				}
				if err := server.Close(); err != nil {
					t.Fatal(err)
				}
				if _, err := readSourcePacket(newSourcePacketReader(client)); !errors.Is(err, io.ErrUnexpectedEOF) {
					t.Fatalf("truncated packet became %v", err)
				}
				return
			}
			done := make(chan error, 1)
			go func() { _, err := readSourcePacket(newSourcePacketReader(client)); done <- err }()
			if err := client.Close(); err != nil {
				t.Fatal(err)
			}
			select {
			case err := <-done:
				if err == nil {
					t.Fatal("closed stream yielded a packet")
				}
			case <-time.After(time.Second):
				t.Fatal("source reader survived channel close")
			}
		})
	}
}

// Same native stream, packets and producer for both paths. A reverse-direction
// token prevents unbounded producer read-ahead. Each op decodes one batch.
func BenchmarkNativeSourcePacketStream(b *testing.B) {
	for _, batch := range []int{1, sourceBoundaryBatchLimit} {
		for _, buffered := range []bool{false, true} {
			b.Run(fmt.Sprintf("batch%d/buffered%t", batch, buffered), func(b *testing.B) {
				server, client := nativePair(b)
				wire := sourceStreamBatch(b, batch)
				done := make(chan error, 1)
				go func() {
					var token [1]byte
					for range b.N {
						if _, err := io.ReadFull(server, token[:]); err != nil {
							done <- err
							return
						}
						if _, err := server.Write(wire); err != nil {
							done <- err
							return
						}
					}
					done <- nil
				}()
				counted := &sourceReadCounter{Reader: client}
				var reader io.Reader = counted
				if buffered {
					reader = newSourcePacketReader(counted)
				}
				var token [1]byte
				b.ReportAllocs()
				b.SetBytes(int64(len(wire)))
				b.ResetTimer()
				for range b.N {
					if _, err := client.Write(token[:]); err != nil {
						b.Fatal(err)
					}
					for i := range batch {
						p, err := readSourcePacket(reader)
						if err != nil || p.sequence != uint64(i+1) {
							b.Fatalf("packet %d: %v", i, err)
						}
					}
				}
				b.StopTimer()
				if err := <-done; err != nil {
					b.Fatal(err)
				}
				b.ReportMetric(float64(counted.calls)/float64(b.N), "reads/op")
			})
		}
	}
}

// Alternate the order within the same run, rather than treating sequential
// benchmark subtests under changing host load as a single-ACK comparison.
func BenchmarkNativeSourcePacketSinglePaired(b *testing.B) {
	server, client := nativePair(b)
	wire := sourceStreamBatch(b, 1)
	done := make(chan error, 1)
	go func() {
		var token [1]byte
		for range 2 * b.N {
			if _, err := io.ReadFull(server, token[:]); err != nil {
				done <- err
				return
			}
			if _, err := server.Write(wire); err != nil {
				done <- err
				return
			}
		}
		done <- nil
	}()
	readers := [2]io.Reader{client, newSourcePacketReader(client)}
	var elapsed [2]time.Duration
	var token [1]byte
	b.ResetTimer()
	for i := range b.N {
		for j := range 2 {
			index := (i + j) % 2
			start := time.Now()
			if _, err := client.Write(token[:]); err != nil {
				b.Fatal(err)
			}
			if _, err := readSourcePacket(readers[index]); err != nil {
				b.Fatal(err)
			}
			elapsed[index] += time.Since(start)
		}
	}
	b.StopTimer()
	if err := <-done; err != nil {
		b.Fatal(err)
	}
	b.ReportMetric(float64(elapsed[0].Nanoseconds())/float64(b.N), "raw-ns/request")
	b.ReportMetric(float64(elapsed[1].Nanoseconds())/float64(b.N), "buffered-ns/request")
}
