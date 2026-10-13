//go:build windows

package wininput

import (
	"context"
	"errors"
	"syscall"
	"testing"
	"unsafe"
)

func TestRawReadBusyRecoversWithoutReplayingVisitor(t *testing.T) {
	for _, busyAt := range []string{"size", "packet"} {
		t.Run(busyAt, func(t *testing.T) {
			r, err := newRawMouseReader()
			if err != nil {
				t.Fatal(err)
			}
			packetSize := uint32(unsafe.Sizeof(rawInputHeader{}) + unsafe.Sizeof(rawMouse{}))
			busy, visits, calls := false, 0, 0
			r.dataCall = func(handle uintptr, data []byte, size *uint32) (uint32, error) {
				calls++
				if handle != 123 {
					t.Fatal("raw input handle changed")
				}
				if !busy && ((busyAt == "size" && data == nil) || (busyAt == "packet" && data != nil)) {
					busy = true
					*size = 999 // A failed in/out size must not leak into the retry.
					return ^uint32(0), syscall.Errno(170)
				}
				if data == nil {
					if *size != 0 {
						t.Fatal("size query was not reset")
					}
					*size = packetSize
					return 0, nil
				}
				if *size != uint32(len(data)) {
					t.Fatal("packet capacity was not reset")
				}
				writeRawMouseBlock(data, 0, rawInputHeader{Type: rimTypeMouse, Size: packetSize}, rawMouse{LastX: 3})
				*size = packetSize
				return packetSize, nil
			}
			if err := r.readCurrentContext(context.Background(), 123, func(_ rawInputHeader, mouse rawMouse) error {
				visits++
				if mouse.LastX != 3 {
					t.Fatal("packet changed")
				}
				return nil
			}); err != nil || calls != 3 || visits != 1 {
				t.Fatal("busy read did not recover exactly once", err, calls, visits)
			}
		})
	}
}

func TestRawReadBusyCannotRetryVisitorOrHidePermanentFailure(t *testing.T) {
	for _, tc := range []struct {
		name   string
		native syscall.Errno
		cancel bool
	}{
		{"busy", 170, false}, {"denied", 5, false}, {"invalid handle", 6, false}, {"canceled busy", 170, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, err := newRawMouseReader()
			if err != nil {
				t.Fatal(err)
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			calls, visits := 0, 0
			r.dataCall = func(uintptr, []byte, *uint32) (uint32, error) {
				calls++
				if tc.cancel {
					cancel()
				}
				return ^uint32(0), tc.native
			}
			err = r.readCurrentContext(ctx, 123, func(rawInputHeader, rawMouse) error { visits++; return nil })
			if err == nil || visits != 0 || calls < 1 || calls > 3 || tc.native != 170 && calls != 1 || tc.cancel && (!errors.Is(err, context.Canceled) || calls != 1) {
				t.Fatal("failed read escaped its native-only budget", err, calls, visits)
			}
			if !tc.cancel && (!errors.Is(err, tc.native) || SourceFailureStage(err) != "raw-size") {
				t.Fatal("original failure lost", err)
			}
		})
	}
	// A visitor may have already routed actions. Its ERROR_BUSY is never replayed.
	r, _ := newRawMouseReader()
	packetSize := uint32(unsafe.Sizeof(rawInputHeader{}) + unsafe.Sizeof(rawMouse{}))
	calls, visits := 0, 0
	r.dataCall = func(_ uintptr, data []byte, size *uint32) (uint32, error) {
		calls++
		*size = packetSize
		if data == nil {
			return 0, nil
		}
		writeRawMouseBlock(data, 0, rawInputHeader{Type: rimTypeMouse, Size: packetSize}, rawMouse{})
		return packetSize, nil
	}
	err := r.readCurrentContext(context.Background(), 123, func(rawInputHeader, rawMouse) error {
		visits++
		return sourceStage("raw-desktop", syscall.Errno(170))
	})
	if !errors.Is(err, syscall.Errno(170)) || SourceFailureStage(err) != "raw-desktop" || calls != 2 || visits != 1 {
		t.Fatal("visitor error was replayed or misclassified", err, calls, visits)
	}
}
