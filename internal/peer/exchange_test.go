package peer

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
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
	if summaries[0].ReadPredecessor != nil {
		t.Fatal("first request invented a preceding dispatch")
	}
}

type slowPreparationResponder struct{ *LeaseResponder }

func (*slowPreparationResponder) HandleInputPreparation(context.Context, time.Time) {
	// Model a preceding handler retaining this serial reader, independently of
	// how fast the following request itself can be authorized and granted.
	time.Sleep(80 * time.Millisecond)
}

func TestReadAheadSeparatesRequestQueueFromReceiverWork(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var summaries []handoff.Summary
		ctx := handoff.WithSession(context.Background(), handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) }))
		defer handoff.EndSession(ctx)
		runtime := newLocalRuntime(t)
		coordinator := testWriteCoordinator(t, &recordingFrameWriter{})
		entry := &recordingCursorEntry{}
		responder := &slowPreparationResponder{&LeaseResponder{Coordinator: coordinator, Runtime: runtime,
			Authorizer: staticLeaseAuthorizer{leaseID: "lease", accepted: true}, Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry}}
		connection := &scriptedConnection{frames: []transport.Frame{
			controlFrame(t, transport.ControlCommand{Kind: transport.ControlPrepareInput}),
			controlFrame(t, transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session", Epoch: "epoch", Edge: control.ScreenEdgeRight}),
			controlFrame(t, transport.ControlCommand{Kind: transport.ControlLeaseAbort, SessionID: "session", Epoch: "epoch", LeaseID: "lease"}),
		}}
		err := runSession(ctx, connection, coordinator, runtime, responder, nil, time.Now, 2*time.Second, nil)
		if !errors.Is(err, io.ErrUnexpectedEOF) || len(summaries) != 1 || !summaries[0].Valid() {
			t.Fatal("queue observation changed protocol", err, summaries)
		}
		s := summaries[0]
		if s.ControlQueue == nil || s.ControlQueue.Micros != 80000 || s.Exchange.Micros[handoff.RequestReadToGrantWritten] != 0 {
			t.Fatal("80ms receiver backlog attributed to transport/authorization", s.ControlQueue, s.Exchange)
		}
	})
}

func TestSessionRequestRetainsSlowPrecedingDispatchWithoutReordering(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		var summaries []handoff.Summary
		collector := handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) })
		ctx := handoff.WithSession(context.Background(), collector)
		defer handoff.EndSession(ctx)
		runtime := newLocalRuntime(t)
		writer := &recordingFrameWriter{}
		coordinator := testWriteCoordinator(t, writer)
		entry := &recordingCursorEntry{}
		responder := &slowPreparationResponder{&LeaseResponder{Coordinator: coordinator, Runtime: runtime,
			Authorizer: staticLeaseAuthorizer{leaseID: "lease", accepted: true}, Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry}}
		prepare := controlFrame(t, transport.ControlCommand{Kind: transport.ControlPrepareInput})
		request := controlFrame(t, transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "session", Epoch: "epoch", Edge: control.ScreenEdgeRight})
		abort := controlFrame(t, transport.ControlCommand{Kind: transport.ControlLeaseAbort, SessionID: "session", Epoch: "epoch", LeaseID: "lease"})
		connection := &scriptedConnection{frames: []transport.Frame{prepare, request, abort}}
		err := RunSessionWithLifecycle(ctx, connection, coordinator, runtime, responder, nil, fixedClock(time.Unix(100, 0)))
		if !errors.Is(err, io.ErrUnexpectedEOF) || len(summaries) != 1 || !summaries[0].Valid() || summaries[0].End != "error" {
			t.Fatal("predecessor evidence changed abort/EOF outcome", err, summaries)
		}
		want := handoff.ReadPredecessorProfile{Schema: 1, Kind: uint8(transport.MessageControl), Micros: [3]uint32{80000, 0, 0}}
		if got := summaries[0].ReadPredecessor; got == nil || *got != want {
			t.Fatal("slow prior dispatch was lost or replaced by the abort", got)
		}
		if got := summaries[0].Exchange; got == nil || got.Micros[handoff.RequestReadToGrantWritten] != 0 {
			t.Fatal("prior dispatch was attributed to the following request", got)
		}
	})
}
