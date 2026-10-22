//go:build windows

package inputworker

const (
	sourceGatePrepare byte = iota + 1
	sourceGatePresence
	sourceGateWrite
)

// Publish only while mu is held; clear before unlocking. No goroutines, clocks,
// polling or additional authority checks enter the writer's critical section.
func (w *sourceWriter) gateStage(origin, stage byte) {
	w.gateOwner.Store(uint32(origin)<<8 | uint32(stage))
}

func (w *sourceWriter) unlockGate() {
	w.gateOwner.Store(0)
	w.mu.Unlock()
}

func (w *sourceWriter) gateFailure(err error) error {
	return sourceTransportFailure(&sourceGateFailure{operation: sourceGateOperation(w.gateOwner.Load()), cause: err})
}
