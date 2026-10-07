package residentagent

import (
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
)

func TestManualDisconnectFencesAutomaticResumeButAllowsExplicitConnect(t *testing.T) {
	for _, active := range []bool{false, true} {
		o := testOperations(t, func(ctx context.Context, _ string, _ agent.SupervisorObserver) error {
			<-ctx.Done()
			return context.Cause(ctx)
		})
		if active {
			if result, err := o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"}); err != nil || result.Status != "COMPLETED" {
				t.Fatal(result, err)
			}
		}
		if _, err := o.Disconnect(context.Background(), localapi.OperationRequest{}); err != nil {
			t.Fatal(err)
		}
		result, err := o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer", AutomaticResume: true})
		if err != nil || result.Status != "REJECTED" || result.Code != "resume_cancelled" {
			t.Fatal("automatic request undid manual stop", active, result, err)
		}
		if status, _ := o.Status(context.Background()); status.State != "LOCAL" {
			t.Fatal(status)
		}
		result, err = o.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
		if err != nil || result.Status != "COMPLETED" {
			t.Fatal("explicit connection was blocked", active, result, err)
		}
		if err = o.Close(context.Background()); err != nil {
			t.Fatal(err)
		}
	}
}
