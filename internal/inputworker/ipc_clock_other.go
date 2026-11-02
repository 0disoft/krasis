//go:build !windows

package inputworker

// Cross-process clock instrumentation is limited to the installed Windows path.
func workerIPCTick() uint64 { return 0 }
