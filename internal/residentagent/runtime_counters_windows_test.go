//go:build windows

package residentagent

import "testing"

func TestRuntimeCountersReadCurrentWindowsProcess(t *testing.T) {
	workingSet, memoryKnown, _, cpuKnown := processRuntimeCounters()
	if !memoryKnown || workingSet == 0 || !cpuKnown {
		t.Fatalf("process counters: memory=%d known=%v cpuKnown=%v", workingSet, memoryKnown, cpuKnown)
	}
}
