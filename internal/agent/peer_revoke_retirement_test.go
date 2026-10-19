package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
)

// Model the installed authorizer's single native lease owner. A logical
// transition to LOCAL alone cannot make a second installed lease available.
type retirementLeaseInput struct {
	*testInput
	ownerMu        sync.Mutex
	owner          control.ReturnIntent
	closes         int
	held           bool
	releases       int
	prematureClose bool
}

func (input *retirementLeaseInput) Press(context.Context, control.PressedToken) error {
	input.ownerMu.Lock()
	defer input.ownerMu.Unlock()
	input.held = true
	return nil
}

func (input *retirementLeaseInput) Release(context.Context, control.PressedToken) error {
	input.ownerMu.Lock()
	defer input.ownerMu.Unlock()
	if input.owner.SessionID == "" {
		return errors.New("native owner retired before key-up")
	}
	input.held = false
	input.releases++
	return nil
}

func (input *retirementLeaseInput) AuthorizeLease(_ context.Context, _ transport.AuthenticatedPeerContext, session, epoch string, _ time.Time) (string, bool, error) {
	input.ownerMu.Lock()
	defer input.ownerMu.Unlock()
	if input.owner.SessionID != "" {
		return "", false, errors.New("installed input admission blocked by existing owner")
	}
	input.owner = control.ReturnIntent{SessionID: session, Epoch: epoch}
	return "lease-" + session, true, nil
}

func (input *retirementLeaseInput) DisarmReturn(intent control.ReturnIntent) {
	input.testInput.DisarmReturn(intent)
	input.ownerMu.Lock()
	defer input.ownerMu.Unlock()
	if input.owner == intent {
		input.prematureClose = input.prematureClose || input.held
		input.owner = control.ReturnIntent{}
		input.closes++
	}
}

func TestRunSessionPeerRevokeRetiresInboundOwnerBeforeNextRequest(t *testing.T) {
	local, remote := openConnectionPair(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_ = remote.SetDeadline(time.Now().Add(3 * time.Second))
	input := &retirementLeaseInput{testInput: newTestInput(false)}
	done := make(chan error, 1)
	go func() {
		_, err := RunSession(ctx, DefaultConfig(control.BoundaryConfig{
			Edge: control.ScreenEdgeRight, ScreenLeft: 0, ScreenWidth: 100, RearmDistance: 10,
		}), Dependencies{Connection: local, Hook: &idleHook{}, Input: input, Effects: noopEffects{}, Authorizer: input})
		done <- err
	}()
	t.Cleanup(func() {
		cancel()
		_ = remote.Close()
		select {
		case <-done:
		case <-time.After(3 * time.Second):
			t.Error("session did not join")
		}
	})
	write := func(command transport.ControlCommand) {
		t.Helper()
		payload, err := transport.EncodeControlPayload(command)
		if err != nil {
			t.Fatal(err)
		}
		if err := remote.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload}); err != nil {
			t.Fatal(err)
		}
	}
	for _, session := range []string{"first", "second"} {
		write(transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: session, Epoch: "epoch", Edge: control.ScreenEdgeRight})
		frame, err := remote.ReadFrame()
		if err != nil {
			t.Fatalf("%s lease grant: %v; peer revocation must retire the previous input owner", session, err)
		}
		grant, err := transport.DecodeControlPayload(frame.Payload)
		if err != nil || grant.Kind != transport.ControlLeaseGrant || grant.SessionID != session {
			t.Fatalf("unexpected grant: %+v, %v", grant, err)
		}
		grant.Kind = transport.ControlLeaseCommit
		write(grant)
		payload, err := transport.EncodeInputPayload(control.InputEnvelope{SessionID: session, Epoch: "epoch", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 65})
		if err != nil {
			t.Fatal(err)
		}
		if err := remote.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageInput, Payload: payload}); err != nil {
			t.Fatal(err)
		}
		grant.Kind = transport.ControlLeaseRevoke
		write(grant)
	}
	// The final read is an ordered barrier: a new request cannot be authorized
	// until the preceding revoke has finished native-owner retirement.
	write(transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "third", Epoch: "epoch", Edge: control.ScreenEdgeRight})
	if _, err := remote.ReadFrame(); err != nil {
		t.Fatal("connection not reusable after second revoke", err)
	}
	input.ownerMu.Lock()
	defer input.ownerMu.Unlock()
	if input.closes != 2 || input.owner.SessionID != "third" || input.releases != 2 || input.prematureClose {
		t.Fatalf("retired=%d, released=%d, premature=%v, owner=%+v", input.closes, input.releases, input.prematureClose, input.owner)
	}
}

func TestManagedPeerRevocationRetiresOnlyMatchingInboundOwner(t *testing.T) {
	for _, direction := range []string{"inbound", "outbound", "stale"} {
		t.Run(direction, func(t *testing.T) {
			ctx := context.Background()
			request := managementTestRequest("revoke", LeaseRevocationOnly)
			input := &retirementLeaseInput{testInput: newTestInput(false), owner: control.ReturnIntent{SessionID: request.Lease.SessionID, Epoch: request.Lease.Epoch}}
			runtime, err := application.NewSessionRuntime(noopEffects{}, input)
			if err != nil {
				t.Fatal(err)
			}
			if direction == "outbound" {
				if _, err := runtime.BeginOutbound(ctx, request.Lease.SessionID, request.Lease.Epoch); err != nil {
					t.Fatal(err)
				}
				if _, err := runtime.GrantOutbound(ctx, request.Lease.SessionID, request.Lease.LeaseID, request.Lease.Epoch, time.Now()); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := runtime.GrantInbound(ctx, control.Event{Kind: control.EventInboundLeaseGranted, SessionID: request.Lease.SessionID, LeaseID: request.Lease.LeaseID}, request.Lease.Epoch, time.Now()); err != nil {
					t.Fatal(err)
				}
			}
			if direction == "stale" {
				request.Lease.LeaseID = "wrong-lease"
			}
			connection := &managementTestConnection{}
			coordinator, err := peer.NewSessionWriteCoordinator(connection)
			if err != nil {
				t.Fatal(err)
			}
			managed, err := newManagedPeerSession(managementTestPeer(), runtime, coordinator, &managementTestSource{}, connection, nil, time.Now, input)
			if err != nil {
				t.Fatal(err)
			}
			err = managed.Revoke(ctx, request)
			if direction == "stale" {
				if !errors.Is(err, ErrManagementLeaseMismatch) {
					t.Fatal(err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			want := 0
			if direction == "inbound" {
				want = 1
			}
			if input.closes != want {
				t.Fatalf("retired=%d, want %d", input.closes, want)
			}
		})
	}
}
