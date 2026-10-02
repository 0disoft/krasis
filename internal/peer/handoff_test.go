package peer

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/transport"
)

func TestHandoffRecordsSourceRequestGrantInputAndMatchingRetirement(t *testing.T) {
	var summaries []handoff.Summary
	ctx := handoff.WithSession(context.Background(), handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) }))
	runtime := newLocalRuntime(t)
	writer := &recordingFrameWriter{}
	coordinator := testWriteCoordinator(t, writer)
	controller, _ := NewOutboundController(coordinator, runtime, &recordingSourceActivation{})
	identity := OutboundIdentity{SessionID: "session", Epoch: "epoch"}
	if err := controller.BeginBoundary(ctx, control.BoundaryIntent{Edge: control.ScreenEdgeRight, DecidedAt: time.Now().Add(-20 * time.Millisecond)}, identity); err != nil {
		t.Fatal(err)
	}
	grant := transport.ControlCommand{Kind: transport.ControlLeaseGrant, SessionID: "session", LeaseID: "lease", Epoch: "epoch"}
	if err := controller.GrantPeer(ctx, grant, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	sink, _ := NewOutboundActionSink(coordinator, runtime, nilClose{})
	if err := sink.SendAction(handoff.Offered(ctx, time.Now().Add(-5*time.Millisecond)), control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, func() bool { return true }); err != nil {
		t.Fatal(err)
	}
	revoke := grant
	revoke.Kind = transport.ControlLeaseRevoke
	if err := controller.RevokePeer(ctx, revoke); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 {
		t.Fatalf("summary count: %d", len(summaries))
	}
	s := summaries[0]
	if !s.Valid() || s.Incomplete || s.Backend != "none" || s.Preparation.Micros[handoff.BoundaryToBegin] < 20000 || s.First.Micros[handoff.SourceQueue] < 5000 || s.First.Outcome != "ok" {
		t.Fatalf("source profile incomplete: %+v", s)
	}
	for _, stage := range []handoff.Stage{handoff.RequestCoordinator, handoff.RequestWrite, handoff.RequestToGrant, handoff.GrantCoordinator, handoff.CommitWrite} {
		if s.Preparation.Measured&(1<<stage) == 0 {
			t.Fatalf("missing stage %d", stage)
		}
	}
}

type nilClose struct{}

func (nilClose) Close() error { return nil }
