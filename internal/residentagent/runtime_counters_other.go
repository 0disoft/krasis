//go:build !windows

package residentagent

func processRuntimeCounters() (uint64, bool, uint64, bool) { return 0, false, 0, false }
