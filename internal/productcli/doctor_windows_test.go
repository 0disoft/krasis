//go:build windows

package productcli

import (
	"context"
	"encoding/binary"
	"testing"
	"unsafe"
)

func TestDoctorServiceStringsStayInsideAlignedBuffer(t *testing.T) {
	words := make([]uint64, 4)
	buffer := unsafe.Slice((*byte)(unsafe.Pointer(&words[0])), len(words)*8)
	binary.LittleEndian.PutUint16(buffer[8:10], 'A')
	ptr := (*uint16)(unsafe.Pointer(&buffer[8]))
	if value, ok := doctorServiceString(buffer, ptr); !ok || value != "A" {
		t.Fatal("native service field rejected")
	}
	var other uint16
	if _, ok := doctorServiceString(buffer, &other); ok {
		t.Fatal("foreign native pointer accepted")
	}
	if _, ok := doctorServiceString(buffer, nil); ok {
		t.Fatal("missing native field accepted")
	}
	for i := 8; i < len(buffer); i += 2 {
		binary.LittleEndian.PutUint16(buffer[i:i+2], 'B')
	}
	if _, ok := doctorServiceString(buffer, ptr); ok {
		t.Fatal("unterminated native field read past buffer")
	}
}

func TestDoctorCancelledHostInspectionRemainsUnknown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	host := inspectDoctorHost(ctx, t.TempDir(), "development")
	if host.Service.State != "UNKNOWN" || host.AgentLocation != "UNKNOWN" {
		t.Fatal("cancelled native checks claimed healthy")
	}
}
