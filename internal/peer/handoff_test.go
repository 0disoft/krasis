package peer

import (
	"context"
	"errors"
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

func TestHandoffCaptureAbortRetiresFailureEvenWhenSessionRecovers(t *testing.T) {
	var summaries []handoff.Summary
	ctx := handoff.WithSession(context.Background(), handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) }))
	runtime := newLocalRuntime(t)
	source := &recordingSourceActivation{activateErr: errors.New("capture unavailable")}
	controller, _ := NewOutboundController(testWriteCoordinator(t, &recordingFrameWriter{}), runtime, source)
	if err := controller.BeginBoundary(ctx, control.BoundaryIntent{Edge: control.ScreenEdgeRight}, OutboundIdentity{SessionID: "session", Epoch: "epoch"}); err != nil {
		t.Fatal(err)
	}
	grant := transport.ControlCommand{Kind: transport.ControlLeaseGrant, SessionID: "session", LeaseID: "lease", Epoch: "epoch"}
	if err := controller.GrantPeer(ctx, grant, time.Unix(100, 0)); err != nil {
		t.Fatal("cleanly recovered abort changed runtime outcome")
	}
	if runtime.Snapshot().Control.State != control.StateLocal || source.active {
		t.Fatal("failed capture remained active")
	}
	if len(summaries) != 1 || summaries[0].End != "error" || !summaries[0].Incomplete {
		t.Fatalf("capture abort not retired: %+v", summaries)
	}
	handoff.EndSession(ctx)
	if len(summaries) != 1 {
		t.Fatal("failure profile emitted twice")
	}
}

func TestHandoffInboundAbortRetiresOnlyMatchingPendingAttempt(t *testing.T) {
	var summaries []handoff.Summary
	ctx := handoff.WithSession(context.Background(), handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) }))
	runtime := newLocalRuntime(t)
	entry := &recordingCursorEntry{}
	responder := &LeaseResponder{Coordinator: testWriteCoordinator(t, &recordingFrameWriter{}), Runtime: runtime,
		Authorizer: staticLeaseAuthorizer{leaseID: "lease", accepted: true}, Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry}
	request := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session", Epoch: "epoch", Edge: control.ScreenEdgeRight}
	if err := responder.HandleLeaseRequest(ctx, request, time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	abort := transport.ControlCommand{Kind: transport.ControlLeaseAbort, SessionID: "old", LeaseID: "lease", Epoch: "epoch"}
	if err := responder.HandleLeaseAbort(ctx, abort); err == nil || len(summaries) != 0 {
		t.Fatal("stale abort retired current profile")
	}
	abort.SessionID = "session"
	if err := responder.HandleLeaseAbort(ctx, abort); err != nil {
		t.Fatal(err)
	}
	if len(summaries) != 1 || summaries[0].End != "error" || runtime.Snapshot().Control.State != control.StateLocal {
		t.Fatalf("matching abort not retired: %+v", summaries)
	}
}
