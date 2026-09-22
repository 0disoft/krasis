package transport

import (
	"fmt"
	"time"
)

type FrameWriteFailure struct {
	Phase string
	Kind  MessageKind
	Wait  time.Duration
	IO    time.Duration
	Cause error
}

func (failure *FrameWriteFailure) Error() string {
	return fmt.Sprintf("peer frame write %s failed: %v", failure.Phase, failure.Cause)
}
func (failure *FrameWriteFailure) Unwrap() error { return failure.Cause }
