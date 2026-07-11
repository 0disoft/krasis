package peer

import (
	"context"
	"errors"
)

type WriteOperation func(FrameWriter) error

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
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-coordinator.permit:
	}
	defer func() { coordinator.permit <- struct{}{} }()
	if err := ctx.Err(); err != nil {
		return err
	}
	return operation(coordinator.writer)
}
