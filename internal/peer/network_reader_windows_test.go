//go:build windows

package peer

import (
	"runtime"
	"testing"
)

func TestNetworkReaderPriorityPreservesPolicyAndRestorationFailure(t *testing.T) {
	for _, before := range []int32{-15, -1, 0, 1, 2, 15, 0x7fffffff} {
		for _, raiseOK := range []bool{false, true} {
			for _, restoreOK := range []bool{false, true} {
				var calls []int32
				restore := networkReaderPriorityScope(func() int32 { return before }, func(value int32) bool {
					calls = append(calls, value)
					if len(calls) == 1 {
						return raiseOK
					}
					return restoreOK
				})
				raised := before == 0 && raiseOK
				if got := restore(); got != (!raised || restoreOK) {
					t.Fatal(before, raiseOK, restoreOK, got)
				}
				want := 0
				if before == 0 {
					want++
				}
				if raised {
					want++
				}
				if len(calls) != want || want > 0 && calls[0] != 1 || raised && calls[1] != 0 {
					t.Fatal(calls)
				}
			}
		}
	}
}

func TestNetworkReaderRestoresRealThreadPriorityBeforeReturning(t *testing.T) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		before := networkThreadPriority()
		if before != 0 {
			t.Errorf("expected normal test thread, got %d", before)
			return
		}
		runNetworkReader(func() {
			if got := networkThreadPriority(); got != 1 {
				t.Errorf("reader priority %d", got)
			}
		})
		if got := networkThreadPriority(); got != before {
			t.Errorf("thread priority leaked: %d", got)
		}
	}()
	<-done
}
