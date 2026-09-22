package wininput

import (
	"testing"
	"time"
)

type timingRecorder struct {
	starts, ends int
	start, end   time.Time
}

func (o *timingRecorder) ObserveHookStarted(t time.Time)   { o.starts++; o.start = t }
func (o *timingRecorder) ObserveHookCompleted(t time.Time) { o.ends++; o.end = t }
func (*timingRecorder) ObserveRawInput(time.Time)          {}
func TestHookTimingPreservesDispositionAndCompletesOnPanic(t *testing.T) {
	o := &timingRecorder{}
	callback := timedSourceCallback(func(code int, w, l uintptr) uintptr { return uintptr(code) + w + l }, o)
	if result := callback(1, 2, 3); result != 6 || o.starts != 1 || o.ends != 1 || o.end.Before(o.start) {
		t.Fatal("callback disposition/timing changed")
	}
	if allocs := testing.AllocsPerRun(100, func() { callback(1, 2, 3) }); allocs != 0 {
		t.Fatalf("callback allocates: %v", allocs)
	}
	func() {
		defer func() {
			if recover() != "test panic" {
				t.Fatal("panic swallowed")
			}
		}()
		timedSourceCallback(func(int, uintptr, uintptr) uintptr { panic("test panic") }, o)(0, 0, 0)
	}()
	if o.starts != o.ends {
		t.Fatal("panic left timing unfinished")
	}
}
