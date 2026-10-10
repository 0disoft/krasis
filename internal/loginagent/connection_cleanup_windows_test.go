//go:build windows

package loginagent

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/wininput"
)

type loginFailedCloseSource struct{}

func (loginFailedCloseSource) Next(ctx context.Context) (agent.Connection, error) {
	return nil, ctx.Err()
}
func (loginFailedCloseSource) Close() error { return context.Canceled }

func TestLoginStopDoesNotHideSupervisorCloseFailureWithCancellationCause(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p, _ := connectionPolicyFixture(t)
	err := agent.RunSupervisor(ctx, agent.SupervisorConfig{Backoff: agent.DefaultBackoffPolicy(), Now: time.Now, Wait: agent.Wait}, agent.SupervisorDependencies{
		Source: loginFailedCloseSource{}, Hook: wininput.NewSourceHook(), Input: wininput.NewInputAdapter(), Effects: agent.RuntimeEffects{}, Authorizer: p, RunSession: agent.RunSession,
	})
	if agent.SupervisorExitPhase(err) != "source-close" || loginStopOnly(err) {
		t.Fatal("source-close failure became a clean stop", err)
	}
}
