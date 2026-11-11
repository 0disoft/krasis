package residentagent

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

type lifecycleCanceledPeer struct {
	stop  context.CancelCauseFunc
	cause error
}

func (c lifecycleCanceledPeer) ReadFrame() (transport.Frame, error) {
	c.stop(c.cause)
	return transport.Frame{}, io.EOF
}
func (lifecycleCanceledPeer) WriteFrameContext(context.Context, transport.Frame) error { return nil }
func (lifecycleCanceledPeer) Close() error                                             { return nil }
func (lifecycleCanceledPeer) AuthenticatedPeer() (transport.AuthenticatedPeerContext, error) {
	return transport.AuthenticatedPeerContext{CertificateSHA256: strings.Repeat("a", 64), Role: transport.AuthenticatedPeerConnector}, nil
}

type lifecycleIdleHook struct{}

func (lifecycleIdleHook) RunWithBoundary(ctx context.Context, _ *wininput.SourceSession, _ wininput.BoundaryObserver, ready func()) error {
	ready()
	<-ctx.Done()
	return ctx.Err()
}

func TestLifecycleCancellationProofCannotHideSemanticCleanupErrors(t *testing.T) {
	bounded, release := context.WithTimeout(context.Background(), 2*time.Second)
	defer release()
	ctx, stop := context.WithCancelCause(bounded)
	defer stop(nil)
	parent := &agentlifecycle.TransitionError{Event: agentlifecycle.EventSessionLock}
	_, proof := agent.RunSession(ctx, agent.DefaultConfig(control.BoundaryConfig{Edge: control.ScreenEdgeRight, ScreenWidth: 100, RearmDistance: 10}), agent.Dependencies{
		Connection: lifecycleCanceledPeer{stop: stop, cause: parent}, Hook: lifecycleIdleHook{},
		Input: wininput.NewInputAdapter(), Effects: agent.RuntimeEffects{}, Authorizer: agent.AuthenticatedPeerAuthorizer{},
	})
	if !agent.RecoveredParentCancellation(proof, parent) || !cancellationOnly(errors.Join(context.Canceled, proof), parent) {
		t.Fatal("actual session cancellation proof rejected", proof)
	}
	unknown := &application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: context.Canceled}
	for _, err := range []error{
		errors.Join(proof, unknown), &application.InjectionError{Cause: proof},
		fmt.Errorf("unverified semantic wrapper: %w", proof),
		errors.Join(proof, application.TokenReleaseError{Err: context.Canceled}),
		errors.Join(proof, &transport.FrameWriteFailure{Cause: context.Canceled}),
		errors.Join(parent, application.TokenReleaseError{Err: context.Canceled}),
		errors.Join(parent, application.EffectFailure{Err: context.Canceled}),
		errors.Join(parent, &transport.FrameWriteFailure{Cause: context.Canceled}),
	} {
		if cancellationOnly(err, parent) {
			t.Fatal("masked extra cleanup failure", err)
		}
	}
}
