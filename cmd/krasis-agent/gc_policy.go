package main

import (
	"os"
	"runtime/debug"
)

// The resident's small live heap otherwise triggers frequent GC stop requests
// while Windows is CPU-saturated. Trade a larger heap budget for fewer cycles.
// This applies only to the resident, not native/capture/guard child processes.
// Explicit GOGC and GOMEMLIMIT operator policies remain authoritative.
const residentGCPercent = 300

func configureResidentGC() {
	if _, configured := os.LookupEnv("GOGC"); !configured {
		debug.SetGCPercent(residentGCPercent)
	}
}
