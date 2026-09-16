package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
	"testing"
	"testing/synctest"
)

func TestRecoveryReportFaultsBeforeRunnerCompletes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		release := make(chan struct{})
		operations := testOperations(t, func(ctx context.Context, _ string, observer agent.SupervisorObserver) error {
			observer.ObserveSupervisor(agent.SupervisorSnapshot{Recovery: &agent.RecoveryReport{DeadlineExceeded: true}})
			<-release
			return context.Canceled
		})
		operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
		synctest.Wait()
		status, _ := operations.Status(context.Background())
		if status.State != "FAULTED" {
			t.Fatalf("pending recovery status: %+v", status)
		}
		if operations.counters["fault_recovery_deadline_exceeded"] != 1 {
			t.Fatal("recovery deadline reason missing")
		}
		result, _ := operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
		if result.Code != "session_failed" {
			t.Fatalf("admitted new owner: %+v", result)
		}
		close(release)
		synctest.Wait()
		status, _ = operations.Status(context.Background())
		if status.State != "FAULTED" {
			t.Fatal("late completion cleared fault latch")
		}
	})
}
