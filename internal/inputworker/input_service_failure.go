package inputworker

import (
	"errors"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/logindiag"
)

type inputServiceFailure struct {
	operation string
	cause     error
}

func (e *inputServiceFailure) Error() string                 { return "input service operation failed" }
func (e *inputServiceFailure) Unwrap() error                 { return e.cause }
func (e *inputServiceFailure) LoginRuntimeOperation() string { return e.operation }

// More specific existing authority/native failures win over a generic phase.
// Wrapping never changes input certainty, recovery fencing or errors.Is.
func inputServiceAt(operation string, cause error) error {
	if cause == nil || !logindiag.ValidRuntimeOperation(operation) {
		return cause
	}
	var detail interface{ LoginRuntimeOperation() string }
	if errors.As(cause, &detail) && logindiag.ValidRuntimeOperation(detail.LoginRuntimeOperation()) && detail.LoginRuntimeOperation() != "" {
		return cause
	}
	return &inputServiceFailure{operation: operation, cause: cause}
}

// Preserve I/O/native causes alongside the existing protocol classification.
// Only these existing fixed frames use this service-local diagnostic helper.
func readInputServiceFrame(reader io.Reader, expected [8]byte) error {
	var actual [8]byte
	if _, err := io.ReadFull(reader, actual[:]); err != nil {
		return errors.Join(ErrProtocol, err)
	}
	if actual != expected {
		return ErrProtocol
	}
	return nil
}

func writeInputServiceFrame(writer io.Writer, data []byte) error {
	n, err := writer.Write(data)
	if err == nil && n != len(data) {
		err = io.ErrShortWrite
	}
	if err != nil {
		return errors.Join(ErrProtocol, err)
	}
	return nil
}

type inputRetirementStream interface {
	io.ReadWriter
	SetDeadline(time.Time) error
}

func retireInputPreparation(stream inputRetirementStream, authenticated, retired [8]byte) error {
	if err := stream.SetDeadline(time.Now().Add(startupTimeout)); err != nil {
		return inputServiceAt("input-retire-deadline", errors.Join(ErrRecovery, err))
	}
	if err := readInputServiceFrame(stream, authenticated); err != nil {
		return inputServiceAt("input-retire-authentication", errors.Join(ErrRecovery, err))
	}
	var stop [bootstrapSize]byte // Same reserved all-zero stop bootstrap.
	if err := writeInputServiceFrame(stream, stop[:]); err != nil {
		return inputServiceAt("input-retire-stop", errors.Join(ErrRecovery, err))
	}
	if err := readInputServiceFrame(stream, retired); err != nil {
		return inputServiceAt("input-retire-receipt", errors.Join(ErrRecovery, err))
	}
	return nil
}

func observeInputPreparationExit(cause error, inspect func() logindiag.WorkerExit) error {
	if cause == nil || inspect == nil {
		return cause
	}
	// Observe the exact owned parent before job cleanup. A process exit alone
	// cannot replace the missing native retirement receipt or clear ErrRecovery.
	exit := inspect()
	if !exit.Valid() {
		return cause
	}
	return &workerExitFailure{cause: cause, exit: exit}
}
