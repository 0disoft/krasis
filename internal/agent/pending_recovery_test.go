package agent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
)

type pendingRecoveryInput struct {
	*testInput
	rollbacks int
}

func (input *pendingRecoveryInput) RollbackEntry(context.Context) (application.CursorRollbackOutcome, error) {
	input.rollbacks++
	return application.CursorRollbackRestored, nil
}

func TestPendingPeerTimeoutClosesConnectionAndRollsBackEntry(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		client, server := openConnectionPair(t)
		defer client.Close()
		defer server.Close()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		input := &pendingRecoveryInput{testInput: newTestInput(false)}
		config := DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10})
		config.Liveness = &LivenessPolicy{HeartbeatInterval: time.Second, PeerTimeout: 2 * time.Second}
		type outcome struct {
			result Result
			err    error
		}
		done := make(chan outcome, 1)
		go func() {
			result, err := RunSession(ctx, config, Dependencies{Connection: server, Hook: &idleHook{}, Input: input, Effects: noopEffects{}, Authorizer: acceptingAuthorizer{}})
			done <- outcome{result, err}
		}()
		payload, err := transport.EncodeControlPayload(transport.ControlCommand{Kind: transport.ControlLeaseRequest, SessionID: "pending", Epoch: "epoch", Edge: control.ScreenEdgeRight})
		if err != nil {
			t.Fatal(err)
		}
		if err := client.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageControl, Payload: payload}); err != nil {
			t.Fatal(err)
		}
		frame, err := client.ReadFrame()
		if err != nil {
			t.Fatal(err)
		}
		grant, err := transport.DecodeControlPayload(frame.Payload)
		if err != nil || grant.Kind != transport.ControlLeaseGrant {
			t.Fatalf("grant: %+v, %v", grant, err)
		}
		// Keep the connection open but never commit or abort the granted lease.
		finished := <-done
		if !errors.Is(finished.err, ErrLeaseHandshakeExpired) || finished.result.FinalState != control.StateLocal || input.rollbacks != 1 {
			t.Fatalf("pending cleanup: %+v, %v, rollbacks=%d", finished.result, finished.err, input.rollbacks)
		}
		grant.Kind = transport.ControlLeaseCommit
		payload, err = transport.EncodeControlPayload(grant)
		if err != nil {
			t.Fatal(err)
		}
		if err := client.WriteFrameContext(context.Background(), transport.Frame{Kind: transport.MessageControl, Payload: payload}); err == nil {
			t.Fatal("expired connection accepted a late commit")
		}
	})
}
