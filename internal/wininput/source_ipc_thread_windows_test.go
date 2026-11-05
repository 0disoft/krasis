//go:build windows

package wininput

import (
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestSourceIPCReaderPriorityLifetimeAndCompletion(t *testing.T) {
	for _, earlyReturn := range []bool{false, true} {
		t.Run(map[bool]string{false: "receive", true: "cancel"}[earlyReturn], func(t *testing.T) {
			type observation struct {
				thread   windows.Handle
				priority int32
				err      error
			}
			observed := make(chan observation, 1)
			input, completed := make(chan struct{}), make(chan struct{})
			go func() {
				defer close(completed)
				runSourceIPCReader(func() {
					h, err := windows.OpenThread(windows.THREAD_QUERY_INFORMATION, false, windows.GetCurrentThreadId())
					observed <- observation{h, currentSourceThreadPriority(), err}
					if earlyReturn {
						return
					}
					<-input
				})
			}()
			var got observation
			select {
			case got = <-observed:
			case <-time.After(5 * time.Second):
				t.Fatal("reader did not start")
			}
			if got.err != nil {
				close(input)
				t.Fatal(got.err)
			}
			defer windows.CloseHandle(got.thread)
			close(input)
			select {
			case <-completed:
			case <-time.After(5 * time.Second):
				t.Fatal("reader lost completion")
			}
			if got.priority != sourceInputPriorityAboveNormal {
				t.Fatal("reader priority", got.priority)
			}
			priority, _, _ := sourceGetThreadPriority.Call(uintptr(got.thread))
			if int32(priority) != sourceInputPriorityNormal {
				t.Fatal("reader did not restore priority before pool reuse", int32(priority))
			}
		})
	}
}

func TestSourceIPCReaderRestorePolicyAndFailure(t *testing.T) {
	for _, before := range []int32{-15, -2, -1, 0, 1, 2, 15, sourceInputPriorityUnknown} {
		for _, raiseOK := range []bool{false, true} {
			for _, restoreOK := range []bool{false, true} {
				var calls []int32
				restore := sourceIPCReaderPriority(func() int32 { return before }, func(p int32) bool {
					calls = append(calls, p)
					if len(calls) == 1 {
						return raiseOK
					}
					return restoreOK
				})
				canUnlock := restore()
				var want []int32
				if before == sourceInputPriorityNormal {
					want = append(want, sourceInputPriorityAboveNormal)
					if raiseOK {
						want = append(want, before)
					}
				}
				if !slices.Equal(calls, want) || canUnlock != (before != sourceInputPriorityNormal || !raiseOK || restoreOK) {
					t.Fatal(before, raiseOK, restoreOK, calls, canUnlock)
				}
			}
		}
	}
}
