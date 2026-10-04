//go:build windows

package inputworker

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestNativeOwnerCycleProbeKeepsPIDAndFailureWithoutInput(t *testing.T) {
	_, client := nativePair(t)
	for _, probe := range []bool{false, true} {
		pid, elapsed, cycles, measured, err := measurePipeOwner(client.handle, probe)
		if err != nil || pid != windows.GetCurrentProcessId() || elapsed < 0 || (!probe && (cycles != 0 || measured)) {
			t.Fatal("optional probe changed native owner result")
		}
		if probe && !measured {
			t.Fatal("native fixture cycle probe failed")
		}
	}
	_, _, _, _, plain := measurePipeOwner(0, false)
	_, _, _, _, probed := measurePipeOwner(0, true)
	if plain == nil || probed != plain {
		t.Fatal("diagnostic probe changed invalid handle failure")
	}
}
