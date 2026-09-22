package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/transport"
)

func TestWriteFailurePhaseSurvivesOuterSessionError(t *testing.T) {
	for _, phase := range []string{"admission", "io", "deadline"} {
		err := errors.Join(peer.ErrOutboundRequestWrite, &transport.FrameWriteFailure{Phase: phase, Kind: transport.MessageControl, Cause: context.DeadlineExceeded})
		descriptor := DescribeSessionFailure(err)
		code := productlog.FaultCode(descriptor.Component, descriptor.Phase, descriptor.Code)
		if code != "fault_write_"+phase+"_timeout" {
			t.Fatalf("lost write phase: %s", code)
		}
	}
}
