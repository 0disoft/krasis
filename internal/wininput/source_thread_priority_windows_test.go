//go:build windows

package wininput

import (
	"errors"
	"testing"
)

func TestSourceInputPriorityHintPreservesExplicitPolicyAndNativeFailure(t *testing.T) {
	for _, current := range []int32{-15, -2, -1, 0, 1, 2, 15, sourceInputPriorityUnknown} {
		for _, succeeds := range []bool{false, true} {
			calls := 0
			applied := promoteSourceInputThread(func() int32 { return current }, func(p int32) bool {
				calls++
				if p != sourceInputPriorityAboveNormal {
					t.Fatal("unbounded input priority", p)
				}
				return succeeds
			})
			want := 0
			if current == sourceInputPriorityNormal {
				want = 1
			}
			if calls != want || applied != (want == 1 && succeeds) {
				t.Fatal(current, succeeds, calls, applied)
			}
		}
	}
}

func TestSourceInputOwnerRunsAtBoundedPriorityAndKeepsOriginalResult(t *testing.T) {
	// Real Windows thread API coverage; no desktop hooks, input or admin action.
	for _, component := range []string{"low-level-hook", "raw-mouse", "wheel-capture"} {
		for _, outcome := range []error{nil, errors.New("original native owner result")} {
			results := make(chan sourceInputThreadResult, 1)
			priority := make(chan int32, 1)
			startSourceInputThread(results, component, func() error {
				priority <- currentSourceThreadPriority()
				return outcome
			})
			result := <-results
			if got := <-priority; got != sourceInputPriorityAboveNormal {
				t.Fatal("owner not raised one level", component, got)
			}
			if result.err != outcome || result.component != component || result.finished.IsZero() {
				t.Fatal("scheduling hint changed native result", result)
			}
		}
	}
}
