package inputworker

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
)

func measureApplyPart(detail *handoff.WorkerApplyProfile, slot int, work func() error) error {
	if detail == nil {
		return work()
	}
	started := time.Now()
	err := work()
	detail.Record(slot, time.Since(started))
	return err
}

// Invoked only inside the existing authorized desktop scope. The flag changes
// measurement and response metadata, never operation order or input authority.
func applyCommand(ctx context.Context, input application.InputPort, pressed *control.PressedState, command request, ack *response) error {
	if command.applyDetail {
		ack.applyDetail = &handoff.WorkerApplyProfile{}
	}
	if command.op == opCursorRead {
		return measureApplyPart(ack.applyDetail, 1, func() error { return readCursorResponse(ctx, input, ack) })
	}
	err := measureApplyPart(ack.applyDetail, 0, func() error { return apply(ctx, input, pressed, command) })
	if err != nil {
		return err
	}
	if command.op == opMoveRead {
		if err := measureApplyPart(ack.applyDetail, 1, func() error { return readCursorResponse(ctx, input, ack) }); err != nil {
			return unknownInput(err)
		}
	}
	return nil
}
