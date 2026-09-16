package residentagent

import (
	"context"
	"github.com/0disoft/krasis/internal/agent"
	"testing"
)

func TestTerminalFaultReasonRecordedWhileDisconnecting(t *testing.T) {
	operations := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error { return nil })
	done := make(chan struct{})
	operations.sessionDone = done
	operations.sessionState = "DISCONNECTING"
	observer := &sessionObserver{operations: operations, done: done}
	observer.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorStopped, FailureKind: "session", FailureComponent: "source-capture", FailurePhase: "capture-activate", FailureCode: "thread_attach_failed"})
	if operations.counters["fault_capture_thread_attach_failed"] != 1 {
		t.Fatal("terminal reason not recorded")
	}
	observer.done = make(chan struct{})
	observer.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorStopped, FailureKind: "session"})
	if operations.counters["fault_session_unknown"] != 0 {
		t.Fatal("stale observer recorded fault")
	}
	operations.sessionDone = nil
}
