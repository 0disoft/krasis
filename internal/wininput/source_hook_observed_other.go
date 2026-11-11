//go:build !windows

package wininput

// Non-Windows hooks preserve the shared constructor contract, but do not emit
// the Windows low-level hook pipeline measurements consumed by this observer.
func NewSourceHookObserved(SourcePipelineObserver) *SourceHook {
	return NewSourceHook()
}
