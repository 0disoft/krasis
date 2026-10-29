package inputworker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestImageHashStartupPreservesBothChildrenAndFrameBoundary(t *testing.T) {
	for _, children := range []bool{false, true} {
		var p handoff.StartupProfile
		p.Record(handoff.WorkerInstalledValidation, 80*time.Millisecond)
		p.ImageHash = handoff.ImageHashProfile{Backend: 1, Micros: [2]uint32{2000, 70000}}
		if children {
			for i := range p.Native {
				p.Native[i] = handoff.NativeStartupProfile{Measured: 31, Micros: [5]uint32{75000, 1000, 2000, 0, 1000}, ImageHash: handoff.ImageHashProfile{Backend: 2, Micros: [2]uint32{1000, 7000}}}
			}
		}
		var wire bytes.Buffer
		if err := writeStartupReady(&wire, p); err != nil {
			t.Fatal(err)
		}
		packet := wire.Bytes()
		if len(packet) != startupReadyImageHashSize || string(packet[:4]) != "KRD9" {
			t.Fatal("hash details not framed")
		}
		reader := bytes.NewReader(append(append([]byte(nil), packet...), 'S'))
		got, err := readStartupReady(reader)
		if err != nil || got == nil || *got != p {
			t.Fatal("hash details changed", got, err)
		}
		if next, err := reader.ReadByte(); err != nil || next != 'S' {
			t.Fatal("hash details consumed next frame")
		}
		for n := startupReadyNativeSize; n < len(packet); n++ {
			if _, err := readStartupReady(bytes.NewReader(packet[:n])); !errors.Is(err, ErrProtocol) {
				t.Fatal("truncated hash profile accepted", n, err)
			}
		}
		for _, offset := range []int{startupReadyNativeSize + 1, startupReadyNativeSize + 13, startupReadyNativeSize + 25} {
			bad := append([]byte(nil), packet...)
			bad[offset] = 1
			if _, err := readStartupReady(bytes.NewReader(bad)); !errors.Is(err, ErrProtocol) {
				t.Fatal("reserved byte accepted", offset)
			}
		}
		bad := append([]byte(nil), packet...)
		binary.BigEndian.PutUint32(bad[startupReadyNativeSize+4:], uint32(handoff.Window.Microseconds())+1)
		reader = bytes.NewReader(append(bad, 'S'))
		if got, err := readStartupReady(reader); err != nil || got != nil {
			t.Fatal("invalid diagnostic affected readiness", got, err)
		}
		if next, err := reader.ReadByte(); err != nil || next != 'S' {
			t.Fatal("invalid diagnostic misaligned following frame")
		}
		var merged handoff.StartupProfile
		mergeWorkerStartup(&merged, &p)
		if merged.ImageHash != p.ImageHash || merged.Native != p.Native {
			t.Fatal("hash details lost in merge")
		}
		if got := handoff.NativeStartupFrom(&handoff.StartupProfile{Schema: 1, Measured: 248, ImageHash: p.ImageHash}); got.ImageHash != p.ImageHash {
			t.Fatal("native receipt lost hash timings")
		}
	}
}
