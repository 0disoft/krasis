//go:build windows

package inputworker

import (
	"io"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

type countedACKReader struct {
	io.Reader
	reads int
}

func (r *countedACKReader) Read(p []byte) (int, error) { r.reads++; return r.Reader.Read(p) }

func TestNativeBufferedACKReadsWholeProfileAndCursorInOneOperation(t *testing.T) {
	server, client := nativePair(t)
	server.SetDeadline(time.Now().Add(time.Second))
	client.SetDeadline(time.Now().Add(time.Second))
	ack := response{op: opCursorEntryProbe, sequence: 1, desktop: inputdesktop.DesktopDefault,
		timing: [3]byte{1, 2, 2}, millis: [3]uint16{0, 1, 2}, measured: 7,
		applyDetail: &handoff.WorkerApplyProfile{Micros: [2]uint32{2000, 0}, Measured: 1, SubmissionMicros: 100, SubmissionCalls: 1},
		x:           10, y: 20, targetX: 10, targetY: 20, beforeX: -3, beforeY: 4}
	if err := writeResponse(server, ack); err != nil {
		t.Fatal(err)
	}
	counter := &countedACKReader{Reader: client}
	got, err := readResponse(newACKReader(counter))
	if err != nil || counter.reads != 1 || got.sequence != 1 || got.desktop != ack.desktop || got.applyDetail == nil || got.applyDetail.SubmissionCalls != 1 || got.beforeX != -3 || got.beforeY != 4 {
		t.Fatalf("native ACK read: calls=%d ack=%+v err=%v", counter.reads, got, err)
	}
}
