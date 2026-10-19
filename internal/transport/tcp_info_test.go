package transport

import (
	"encoding/binary"
	"testing"
)

func TestTCPInfoV0UsesWindowsABIOffsetsAndRejectsPartialRecords(t *testing.T) {
	var data [88]byte
	for i := range data {
		data[i] = 0xff
	}
	for i, offset := range []int{20, 24, 68, 80} {
		binary.LittleEndian.PutUint32(data[offset:offset+4], uint32(i+1))
	}
	if got, ok := decodeTCPInfoV0(data[:]); !ok || got != ([4]uint32{1, 2, 3, 4}) {
		t.Fatal("TCP_INFO_v0 padding or field mapping changed", got, ok)
	}
	for _, bad := range [][]byte{nil, data[:84], append(data[:], 0)} {
		if got, ok := decodeTCPInfoV0(bad); ok || got != ([4]uint32{}) {
			t.Fatal("partial/unknown structure became statistics", got, ok)
		}
	}
	for _, connection := range []*PeerConnection{nil, {}} {
		if got, ok := connection.DiagnosticTCPInfo(); ok || got != ([4]uint32{}) {
			t.Fatal("missing connection became an observation")
		}
	}
}
