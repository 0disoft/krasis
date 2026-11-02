//go:build windows

package inputworker

import (
	"bytes"
	"errors"
	"os"
	"testing"
	"time"
)

type signalledServiceWriter struct {
	*pipeConn
	entered chan struct{}
}

func (w *signalledServiceWriter) writeServiceResponse(data []byte, offset int) (int, error) {
	close(w.entered) // Serialization is complete; native writer admission follows.
	return w.pipeConn.writeServiceResponse(data, offset)
}

func TestNativeServiceSubmissionSeparatesWriterAdmissionFromDelivery(t *testing.T) {
	server, client := nativePair(t)
	if workerIPCTick() == 0 {
		t.Skip("same-host QPC unavailable")
	}
	if err := server.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	if err := client.SetDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatal(err)
	}
	w := &signalledServiceWriter{pipeConn: server, entered: make(chan struct{})}
	server.writeMu.Lock()
	done := make(chan error, 1)
	go func() {
		done <- writeResponse(w, response{sequence: 1, op: opMove,
			ipc: &nativeIPCWire{outer: [3]uint64{workerIPCTick(), 0, 0}}})
	}()
	<-w.entered
	time.Sleep(20 * time.Millisecond)
	server.writeMu.Unlock()
	if err := <-done; err != nil {
		t.Fatal(err)
	}
	ack, err := readResponse(client)
	if err != nil || ack.ipc == nil || !ack.ipc.nativeWrite || ack.ipc.outer[2] < ack.ipc.outer[1]+20000 {
		t.Fatal("native stamp hid writer admission", ack.ipc, err)
	}
}

func TestNativeServiceSubmissionDoesNotPatchExpiredOrInvalidWrites(t *testing.T) {
	server, _ := nativePair(t)
	var wire bytes.Buffer
	if err := writeResponse(&wire, response{sequence: 1, op: opMove,
		ipc: &nativeIPCWire{outer: [3]uint64{workerIPCTick(), 0, 0}}}); err != nil {
		t.Fatal(err)
	}
	data := wire.Bytes()
	offset := len(data) - nativeIPCSize
	if n, err := server.writeServiceResponse(data, -1); n != 0 || !errors.Is(err, ErrProtocol) {
		t.Fatal("invalid ACK stamp offset reached writer", n, err)
	}
	if err := server.SetWriteDeadline(time.Now().Add(-time.Millisecond)); err != nil {
		t.Fatal(err)
	}
	if n, err := server.writeServiceResponse(data, offset); n != 0 || !errors.Is(err, os.ErrDeadlineExceeded) || data[offset+1]&4 != 0 {
		t.Fatal("expired write claimed native submission", n, err)
	}
}
