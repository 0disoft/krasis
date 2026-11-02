package inputworker

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestNativeIPCNormalizesOnlyOrderedBoundedSameExchange(t *testing.T) {
	wire := &nativeIPCWire{stamps: [3]uint64{140, 150, 300}}
	p := normalizeNativeIPC(100, 110, 120, wire, 500, 510)
	if p == nil || p.Micros != [6]uint32{20, 20, 10, 150, 200, 10} || !p.Valid() {
		t.Fatal("IPC phase boundaries lost", p)
	}
	for _, stamps := range [][3]uint64{{0, 150, 300}, {119, 150, 300}, {140, 139, 300}, {140, 150, 501}, {^uint64(0), ^uint64(0), ^uint64(0)}} {
		if normalizeNativeIPC(100, 110, 120, &nativeIPCWire{stamps: stamps}, 500, 510) != nil {
			t.Fatal("foreign clock or unordered timestamps accepted", stamps)
		}
	}
	limit := uint64(handoff.Window.Microseconds())
	if normalizeNativeIPC(100, 110, 120, wire, 500, 100+limit) == nil || normalizeNativeIPC(100, 110, 120, wire, 500, 101+limit) != nil {
		t.Fatal("IPC observation window boundary changed")
	}
}

func TestNativeIPCWireKeepsCursorDesktopAndNextACKAligned(t *testing.T) {
	for _, desktop := range []inputdesktop.DesktopClass{inputdesktop.DesktopUnknown, inputdesktop.DesktopDefault, inputdesktop.DesktopWinlogon} {
		var wire bytes.Buffer
		profile := &handoff.NativeIPCProfile{Micros: [6]uint32{1, 2, 3, 4, 5, 6}}
		ack := response{sequence: 1, op: opCursorEntryProbe, desktop: desktop, x: 7, beforeX: -9,
			applyDetail: &handoff.WorkerApplyProfile{Micros: [2]uint32{100, 0}, Measured: 1, NativeIPC: profile}}
		if err := writeResponse(&wire, ack); err != nil {
			t.Fatal(err)
		}
		if err := writeResponse(&wire, response{sequence: 2, op: opMove}); err != nil {
			t.Fatal(err)
		}
		reader := newACKReader(&wire)
		got, err := readResponse(reader)
		if err != nil || got.sequence != 1 || got.x != 7 || got.beforeX != -9 || got.desktop != desktop || got.applyDetail == nil || got.applyDetail.NativeIPC == nil || *got.applyDetail.NativeIPC != *profile {
			t.Fatal("IPC suffix lost cursor/desktop or profile", got, err)
		}
		if got, err := readResponse(reader); err != nil || got.sequence != 2 {
			t.Fatal("IPC suffix consumed following ACK", got, err)
		}
	}
}

func TestNativeIPCBadNumbersDropMetadataWithoutChangingInputResult(t *testing.T) {
	var wire bytes.Buffer
	ack := response{sequence: 1, op: opMove, ipc: &nativeIPCWire{stamps: [3]uint64{10, 20, 30}}}
	if err := writeResponse(&wire, ack); err != nil {
		t.Fatal(err)
	}
	data := wire.Bytes()
	data[responseSize] = 2
	binary.BigEndian.PutUint32(data[responseSize+8:], uint32(handoff.Window.Microseconds()+1))
	got, err := readResponse(bytes.NewReader(data))
	if err != nil || got.status != 0 || got.ipc != nil {
		t.Fatal("invalid timing changed acknowledged input", got, err)
	}
	if _, err := readResponse(bytes.NewReader(data[:len(data)-1])); err == nil {
		t.Fatal("truncated IPC frame accepted")
	}
}

func TestNativeIPCRequestRequiresExistingApplyAndSubmissionProbe(t *testing.T) {
	var binding Binding
	binding[0] = 1
	for _, valid := range []bool{false, true} {
		r := request{op: opMove, sequence: 1, binding: binding, x: 1, applyDetail: true, submissionDetail: valid, ipcDetail: true}
		data, err := encodeRequest(r)
		if !valid {
			if err == nil {
				t.Fatal("IPC admitted without submission profile")
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		got, err := readRequest(bytes.NewReader(data[:]))
		if err != nil || !got.ipcDetail || !got.submissionDetail {
			t.Fatal(got, err)
		}
		data[7] = 5
		if _, err := readRequest(bytes.NewReader(data[:])); err == nil {
			t.Fatal("unknown probe flags accepted")
		}
	}
}

func TestNativeIPCShortWritesRemainFailures(t *testing.T) {
	w := &shortIPCWriter{}
	if err := writeResponse(w, response{sequence: 1, op: opMove, ipc: &nativeIPCWire{}}); err != io.ErrShortWrite || w.calls != 1 {
		t.Fatal("IPC metadata changed single-write failure", err, w.calls)
	}
}

type shortIPCWriter struct{ calls int }

func (w *shortIPCWriter) Write(data []byte) (int, error) { w.calls++; return len(data) - 1, nil }
