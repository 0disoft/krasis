//go:build windows

package wininput

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestAtomicCapturePublicationKeepsSingleOwnerAndRejectsStaleDetach(t *testing.T) {
	capture := &sourceWheelCapture{}
	var winners atomic.Int32
	var workers sync.WaitGroup
	for i := 0; i < 32; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if capture.attach(&sourceWheelCaptureRun{}) == nil {
				winners.Add(1)
			}
		}()
	}
	workers.Wait()
	if winners.Load() != 1 || capture.run.Load() == nil {
		t.Fatal("concurrent native owners admitted")
	}
	first := capture.run.Load()
	capture.detach(first)
	second := &sourceWheelCaptureRun{window: 99}
	second.active.Store(true)
	if capture.attach(second) != nil {
		t.Fatal("retired owner blocked replacement")
	}
	capture.detach(first)
	oldForeground := sourceWheelCaptureGetForeground
	t.Cleanup(func() { sourceWheelCaptureGetForeground = oldForeground })
	sourceWheelCaptureGetForeground = func() uintptr { return 99 }
	if capture.run.Load() != second || !capture.Active() || !capture.OwnsWheel() {
		t.Fatal("stale detach removed current wheel owner")
	}
	second.active.Store(false)
	if capture.Active() || capture.OwnsWheel() {
		t.Fatal("deactivated owner retained wheel suppression")
	}
	capture.detach(second)
	if capture.Active() || capture.OwnsWheel() {
		t.Fatal("detached owner retained wheel suppression")
	}
	if capture.attach(nil) == nil {
		t.Fatal("nil native owner admitted")
	}
}
