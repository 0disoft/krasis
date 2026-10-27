//go:build windows

package inputworker

import (
	"context"
	"errors"
	"time"
)

// The source-session sink interrupts input, not the shared reply/retirement
// transport. Serialize deadline setup with that interruption so a sampled old
// action cannot expire a later control reply. Native I/O never holds this gate.
func (w *sourceWriter) writeFrame(data []byte, deadline time.Time, origin byte) error {
	w.interruptMu.Lock()
	if origin == sourceAction && w.inputStopped {
		w.interruptMu.Unlock()
		// Admission stopped before Write: no sequence or stream was consumed.
		return &sourceIOFailure{stage: "deadline", cause: errors.Join(ErrAuthority, context.Canceled)}
	}
	err := w.channel.stream.SetWriteDeadline(deadline)
	if err == nil {
		w.writeOrigin = origin
	}
	w.interruptMu.Unlock()
	if err != nil {
		return &sourceIOFailure{stage: "deadline", cause: errors.Join(ErrAuthority, err)}
	}
	defer func() {
		w.interruptMu.Lock()
		w.writeOrigin = 0
		w.interruptMu.Unlock()
	}()
	return writeSourceBytes(w.channel.stream, data)
}

func (w *sourceWriter) Interrupt(error) error {
	w.interruptMu.Lock()
	defer w.interruptMu.Unlock()
	w.inputStopped = true
	if w.writeOrigin != sourceAction {
		return nil
	}
	return sourceTransportFailure(w.channel.stream.SetWriteDeadline(time.Now()))
}
