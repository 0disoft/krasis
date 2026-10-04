package peer

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/transport"
)

func TestSessionGrantExchangeUsesReadBoundaryRatherThanPolicyClock(t *testing.T) {
	var summaries []handoff.Summary
	collector := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	ctx := handoff.WithSession(context.Background(), collector)
	t.Cleanup(func() { handoff.EndSession(ctx) })
	runtime := newLocalRuntime(t)
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	controller, _ := NewOutboundController(coordinator, runtime, &recordingSourceActivation{})
	if err := controller.BeginBoundary(ctx, control.BoundaryIntent{Edge: control.ScreenEdgeRight}, OutboundIdentity{SessionID: "session", Epoch: "epoch"}); err != nil {
		t.Fatal(err)
	}
	grant := transport.ControlCommand{Kind: transport.ControlLeaseGrant, SessionID: "session", LeaseID: "lease", Epoch: "epoch"}
	revoke := grant
	revoke.Kind = transport.ControlLeaseRevoke
	connection := &scriptedConnection{frames: []transport.Frame{controlFrame(t, grant), controlFrame(t, revoke)}}
	err := RunSessionWithLifecycle(ctx, connection, coordinator, runtime, nil, controller, fixedClock(time.Unix(100, 0)))
	if !errors.Is(err, io.ErrUnexpectedEOF) || len(summaries) != 1 || !summaries[0].Valid() {
		t.Fatal("exchange changed session outcome", err, summaries)
	}
	p := summaries[0].Exchange
	if p == nil || p.Measured != 3 || p.Micros[handoff.GrantReadToHandler] == uint32(handoff.Window.Microseconds()) {
		t.Fatal("read boundary lost or injectable policy clock used for elapsed time", p)
	}
}

func TestSessionRequestExchangePreservesMatchingAbort(t *testing.T) {
	var summaries []handoff.Summary
	collector := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
	ctx := handoff.WithSession(context.Background(), collector)
	t.Cleanup(func() { handoff.EndSession(ctx) })
	runtime := newLocalRuntime(t)
	coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
	entry := &recordingCursorEntry{}
	responder := &LeaseResponder{Coordinator: coordinator, Runtime: runtime,
		Authorizer: staticLeaseAuthorizer{leaseID: "lease", accepted: true}, Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry}
	request := transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session", Epoch: "epoch", Edge: control.ScreenEdgeRight}
	abort := transport.ControlCommand{Kind: transport.ControlLeaseAbort, SessionID: "session", Epoch: "epoch", LeaseID: "lease"}
	connection := &scriptedConnection{frames: []transport.Frame{controlFrame(t, request), controlFrame(t, abort)}}
	err := RunSessionWithLifecycle(ctx, connection, coordinator, runtime, responder, nil, fixedClock(time.Unix(100, 0)))
	if !errors.Is(err, io.ErrUnexpectedEOF) || len(summaries) != 1 || !summaries[0].Valid() || summaries[0].End != "error" {
		t.Fatal("matching abort or diagnostic outcome changed", err, summaries)
	}
	p := summaries[0].Exchange
	if p == nil || p.Measured != 12 || p.Micros[handoff.RequestReadToHandler] > p.Micros[handoff.RequestReadToGrantWritten] {
		t.Fatal("receiver read boundary lost or overlapping spans reversed", p)
	}
}
