//go:build windows

package wininput

import (
	"strings"
	"testing"
	"unsafe"
)

func TestAlignedRawInputBufferIsEightByteAligned(t *testing.T) {
	buffer, err := newAlignedRawInputBuffer(4096)
	if err != nil {
		t.Fatal(err)
	}
	if buffer.pointer()%rawInputBlockAlignment != 0 {
		t.Fatalf("buffer pointer %#x is not %d-byte aligned", buffer.pointer(), rawInputBlockAlignment)
	}
}

func TestRawInputWindowCleanupReportsInvalidHandle(t *testing.T) {
	// No window, hook, input registration or desktop mutation is created.
	err := sourceStage("raw-window-destroy", destroyRawInputMessageWindow(0))
	if err == nil || SourceFailureStage(err) != "raw-window-destroy" {
		t.Fatal("failed raw window cleanup was hidden", err)
	}
}

func TestVisitRawInputBlocksTraversesAlignedMousePackets(t *testing.T) {
	headerSize := uintptr(unsafe.Sizeof(rawInputHeader{}))
	packetSize := headerSize + unsafe.Sizeof(rawMouse{})
	stride := alignRawInputBlock(packetSize)
	buffer := make([]byte, stride*2)
	writeRawMouseBlock(buffer, 0, rawInputHeader{Type: rimTypeMouse, Size: uint32(packetSize), Device: 0}, rawMouse{LastX: -4})
	writeRawMouseBlock(buffer, stride, rawInputHeader{Type: rimTypeMouse, Size: uint32(packetSize), Device: 9}, rawMouse{LastY: 6})

	var got []rawMouse
	err := visitRawInputBlocks(buffer, 2, func(_ rawInputHeader, mouse rawMouse) error {
		got = append(got, mouse)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].LastX != -4 || got[1].LastY != 6 {
		t.Fatalf("visited packets = %+v", got)
	}
}

func TestVisitRawInputBlocksRejectsTruncatedAndZeroSizedPackets(t *testing.T) {
	headerSize := uintptr(unsafe.Sizeof(rawInputHeader{}))
	buffer := make([]byte, headerSize)
	*(*rawInputHeader)(unsafe.Pointer(&buffer[0])) = rawInputHeader{Type: rimTypeMouse, Size: 0}
	err := visitRawInputBlocks(buffer, 1, func(rawInputHeader, rawMouse) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "invalid size") {
		t.Fatalf("zero-sized block error = %v", err)
	}

	packetSize := headerSize + unsafe.Sizeof(rawMouse{})
	buffer = make([]byte, packetSize-1)
	*(*rawInputHeader)(unsafe.Pointer(&buffer[0])) = rawInputHeader{Type: rimTypeMouse, Size: uint32(packetSize)}
	err = visitRawInputBlocks(buffer, 1, func(rawInputHeader, rawMouse) error { return nil })
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("truncated block error = %v", err)
	}
}

func writeRawMouseBlock(buffer []byte, offset uintptr, header rawInputHeader, mouse rawMouse) {
	pointer := unsafe.Pointer(&buffer[offset])
	*(*rawInputHeader)(pointer) = header
	*(*rawMouse)(unsafe.Add(pointer, unsafe.Sizeof(rawInputHeader{}))) = mouse
}
