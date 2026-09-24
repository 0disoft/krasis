package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/wininput"
	"testing"
)

func TestFinalRunnerErrorCannotLoseFaultReason(t *testing.T) {
	o := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error { return nil })
	done := make(chan struct{})
	outcome := &sessionOutcome{}
	o.sessionDone, o.sessionOutcome = done, outcome
	o.finishSession(done, outcome, errors.Join(wininput.ErrSourceCaptureLostForeground, errors.New("private native details")), false, false)
	if o.counters["fault_capture_foreground_lost"] != 1 || o.counters["session_failed"] != 1 {
		t.Fatalf("final cause omitted: %+v", o.counters)
	}
}

func TestFinalFaultDeduplicatesObserverAndIgnoresStaleSession(t *testing.T) {
	o := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error { return nil })
	done := make(chan struct{})
	outcome := &sessionOutcome{}
	o.sessionDone, o.sessionOutcome = done, outcome
	observer := &sessionObserver{operations: o, done: done}
	observer.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorStopped, FailureKind: "session", FailureComponent: "source-capture", FailurePhase: "capture-runtime", FailureCode: "foreground_lost"})
	o.finishSession(done, outcome, wininput.ErrSourceCaptureLostForeground, false, false)
	if o.counters["fault_capture_foreground_lost"] != 1 {
		t.Fatal("same fault counted twice")
	}
	o.sessionDone = make(chan struct{})
	stale := make(chan struct{})
	o.finishSession(stale, &sessionOutcome{}, wininput.ErrSourceCaptureCommandTimeout, false, false)
	if o.counters["fault_capture_command_timeout"] != 0 {
		t.Fatal("old session polluted current diagnostics")
	}
	o.sessionDone = nil
}

func TestNormalCancellationDoesNotRecordFault(t *testing.T) {
	o := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error { return nil })
	done := make(chan struct{})
	o.sessionDone = done
	o.finishSession(done, &sessionOutcome{}, context.Canceled, true, false)
	if o.counters["fault_session_unknown"] != 0 || o.counters["session_stopped"] != 1 {
		t.Fatal(o.counters)
	}
}

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
