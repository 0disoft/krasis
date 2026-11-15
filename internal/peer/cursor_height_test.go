package peer

import (
	"context"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
	"testing"
	"time"
)

type heightFrameWriter struct {
	recordingFrameWriter
	supported bool
}

func (w *heightFrameWriter) SupportsCursorHeight() bool { return w.supported }

type heightSource struct {
	recordingSourceActivation
	height control.EdgeHeight
}

func (s *heightSource) DeactivateProfile(ctx context.Context) {
	s.height = control.EdgeHeightFromContext(ctx)
	s.Deactivate()
}

type heightEntry struct {
	recordingCursorEntry
	height control.EdgeHeight
}

func (e *heightEntry) PrepareEntry(ctx context.Context, edge control.ScreenEdge) error {
	e.height = control.EdgeHeightFromContext(ctx)
	return e.recordingCursorEntry.PrepareEntry(ctx, edge)
}

func TestCursorHeightCrossingAndReturnPipeline(t *testing.T) {
	for _, supported := range []bool{false, true} {
		for _, h := range []control.EdgeHeight{1, 32769, 65536} {
			ctx := context.Background()
			aRuntime := newLocalRuntime(t)
			aw := &heightFrameWriter{supported: supported}
			as := &heightSource{}
			a, err := NewOutboundController(testWriteCoordinator(t, aw), aRuntime, as)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.BeginBoundary(ctx, control.BoundaryIntent{Edge: control.ScreenEdgeRight, Height: h}, OutboundIdentity{SessionID: "session-1", Epoch: "epoch-1"}); err != nil {
				t.Fatal(err)
			}
			request, err := transport.DecodeControlPayload(aw.frames[0].Payload)
			if err != nil {
				t.Fatal(err)
			}
			want := h
			if !supported {
				want = 0
			}
			if request.Height != want {
				t.Fatal("outbound height lost", request)
			}
			br := newLocalRuntime(t)
			bw := &heightFrameWriter{supported: supported}
			entry := &heightEntry{recordingCursorEntry: recordingCursorEntry{runtime: br}}
			responder := &LeaseResponder{Coordinator: testWriteCoordinator(t, bw), Runtime: br, Authorizer: staticLeaseAuthorizer{leaseID: "lease-1", accepted: true}, Peer: testAuthenticatedPeer(), Entry: entry, Returns: entry}
			if err := responder.HandleLeaseRequest(ctx, request, time.Unix(100, 0)); err != nil {
				t.Fatal(err)
			}
			if entry.height != want {
				t.Fatal("entry context lost height", entry.height)
			}
			grant, err := transport.DecodeControlPayload(bw.frames[0].Payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.GrantPeer(ctx, grant, time.Unix(100, 0)); err != nil {
				t.Fatal(err)
			}
			// Independently exercise an active receiver's natural return at a
			// different height: returning must not reuse the original entry.
			inbound, _ := newInboundRuntime(t)
			rw := &heightFrameWriter{supported: supported}
			returns, err := NewReturnController(testWriteCoordinator(t, rw), inbound, &recordingReturnBoundary{})
			if err != nil {
				t.Fatal(err)
			}
			back := control.EdgeHeight(65537 - h)
			if err := returns.Revoke(ctx, control.ReturnIntent{SessionID: "session-1", Epoch: "epoch-1", Height: back}); err != nil {
				t.Fatal(err)
			}
			revoke, err := transport.DecodeControlPayload(rw.frames[0].Payload)
			if err != nil {
				t.Fatal(err)
			}
			if err := a.RevokePeer(ctx, revoke); err != nil {
				t.Fatal(err)
			}
			wantBack := back
			if !supported {
				wantBack = 0
			}
			if as.height != wantBack || as.active {
				t.Fatal("return height lost or capture remained active", as)
			}
		}
	}
}

func TestStaleRevocationCannotRepositionCursor(t *testing.T) {
	r := newOutboundRuntime(t)
	if _, err := r.GrantOutbound(context.Background(), "session-1", "lease-1", "epoch-1", time.Unix(100, 0)); err != nil {
		t.Fatal(err)
	}
	s := &heightSource{recordingSourceActivation: recordingSourceActivation{active: true}}
	c, err := NewOutboundController(testWriteCoordinator(t, &recordingFrameWriter{}), r, s)
	if err != nil {
		t.Fatal(err)
	}
	err = c.RevokePeer(context.Background(), transport.ControlCommand{Kind: transport.ControlLeaseRevoke, SessionID: "session-1", LeaseID: "lease-1", Epoch: "stale", Height: 65536})
	if err == nil || s.height != 0 {
		t.Fatal("stale lease moved cursor", err, s.height)
	}
}
