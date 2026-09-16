package residentagent

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"testing/synctest"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
)

func TestFailedRunnerLatchesFaultAndRejectsNewWork(t *testing.T) {
	for _, failure := range []error{nil, errors.New("private cleanup failure")} {
		synctest.Test(t, func(t *testing.T) {
			calls := 0
			operations := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error {
				calls++
				return failure
			})
			operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
			synctest.Wait()
			for range 2 {
				status, _ := operations.Status(context.Background())
				if status.State != "FAULTED" || !reflect.DeepEqual(status.Actions, []string{"SAFE_STOP"}) {
					t.Fatalf("unsafe status: %+v", status)
				}
				result, _ := operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
				if result.Status != "REJECTED" || result.Code != "session_failed" || calls != 1 {
					t.Fatalf("fault allowed connection: %+v calls=%d", result, calls)
				}
				result, _ = operations.Pair(context.Background(), localapi.OperationRequest{})
				if result.Status != "REJECTED" || result.Code != "session_failed" {
					t.Fatalf("fault allowed pairing: %+v", result)
				}
				operations.Disconnect(context.Background(), localapi.OperationRequest{})
			}
		})
	}
}

func TestLocalDisconnectCleanupFailureDoesNotBecomeLocal(t *testing.T) {
	for _, failure := range []error{nil, context.DeadlineExceeded, errors.New("release failed")} {
		synctest.Test(t, func(t *testing.T) {
			operations := testOperations(t, func(ctx context.Context, _ string, _ agent.SupervisorObserver) error {
				<-ctx.Done()
				return errors.Join(ctx.Err(), failure)
			})
			operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
			result, _ := operations.Disconnect(context.Background(), localapi.OperationRequest{})
			status, _ := operations.Status(context.Background())
			if failure == nil {
				if result.Status != "COMPLETED" || status.State != "LOCAL" {
					t.Fatalf("clean stop faulted: %+v %+v", result, status)
				}
			} else if result.Status != "FAILED" || status.State != "FAULTED" {
				t.Fatalf("cleanup failure hidden: %+v %+v", result, status)
			}
		})
	}
}
