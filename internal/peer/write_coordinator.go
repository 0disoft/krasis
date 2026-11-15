package peer

import (
	"context"
	"errors"
	"runtime/trace"

	"github.com/0disoft/krasis/internal/control"
)

type WriteOperation func(FrameWriter) error

func cursorHeightForWriter(writer FrameWriter, height control.EdgeHeight) control.EdgeHeight {
	if capability, ok := writer.(interface{ SupportsCursorHeight() bool }); ok && capability.SupportsCursorHeight() {
		return height
	}
	return 0
}

// SessionWriteCoordinator serializes runtime decisions with their semantic
// frame writes for one authenticated peer connection.
type SessionWriteCoordinator struct {
	writer FrameWriter
	permit chan struct{}
}

func NewSessionWriteCoordinator(writer FrameWriter) (*SessionWriteCoordinator, error) {
	if writer == nil {
		return nil, errors.New("session frame writer is required")
	}
	permit := make(chan struct{}, 1)
	permit <- struct{}{}
	return &SessionWriteCoordinator{writer: writer, permit: permit}, nil
}

func (coordinator *SessionWriteCoordinator) Execute(ctx context.Context, operation WriteOperation) error {
	if coordinator == nil || coordinator.writer == nil || coordinator.permit == nil {
		return errors.New("session write coordinator is not initialized")
	}
	if ctx == nil || operation == nil {
		return errors.New("session write context and operation are required")
	}
	waitRegion := trace.StartRegion(ctx, "krasis.peer.coordinator_wait")
	select {
	case <-ctx.Done():
		waitRegion.End()
		return ctx.Err()
	case <-coordinator.permit:
	}
	waitRegion.End()
	defer func() { coordinator.permit <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	operationRegion := trace.StartRegion(ctx, "krasis.peer.coordinator_operation")
	defer operationRegion.End()
	return operation(coordinator.writer)
}
