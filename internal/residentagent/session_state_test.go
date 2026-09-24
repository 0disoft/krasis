package residentagent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/wininput"
	"testing"
	"testing/synctest"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/localapi"
)

func TestDisplayChangeStopReasonRequiresCleanPreviouslyConnectedSession(t *testing.T) {
	for _, test := range []struct {
		name, wantState, wantReason string
		connected, manualStop       bool
		err                         error
		cancelled                   bool
	}{
		{"clean display change", "LOCAL", "display_change", true, false, errors.Join(context.Canceled, wininput.ErrDisplayGeometryChanged), false},
		{"never connected", "FAULTED", "", false, false, errors.Join(context.Canceled, wininput.ErrDisplayGeometryChanged), false},
		{"cleanup failure", "FAULTED", "", true, false, errors.Join(context.Canceled, wininput.ErrDisplayGeometryChanged, errors.New("cleanup failed")), false},
		{"manual disconnect", "LOCAL", "", true, true, errors.Join(context.Canceled, wininput.ErrDisplayGeometryChanged), true},
	} {
		t.Run(test.name, func(t *testing.T) {
			operations := testOperations(t, nil)
			done := make(chan struct{})
			outcome := &sessionOutcome{connected: test.connected}
			operations.sessionDone = done
			operations.sessionOutcome = outcome
			operations.peerAlias = "B"
			operations.manualStop = test.manualStop
			operations.finishSession(done, outcome, test.err, test.cancelled, displayChangeOnly(test.err))
			status, err := operations.Status(context.Background())
			if err != nil || status.State != test.wantState || status.StopReason != test.wantReason || status.PeerAlias != "" {
				t.Fatalf("status = %+v, %v", status, err)
			}
		})
	}
}

func TestSessionStateUsesCurrentSupervisorAndCleanupCompletion(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		observers := make(chan agent.SupervisorObserver, 2)
		releases := make(chan chan struct{}, 2)
		operations := testOperations(t, func(ctx context.Context, _ string, observer agent.SupervisorObserver) error {
			release := make(chan struct{})
			releases <- release
			observers <- observer
			<-ctx.Done()
			<-release
			return context.Cause(ctx)
		})
		connect := func() {
			t.Helper()
			result, err := operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"})
			if err != nil || result.Code != "connect_started" {
				t.Fatalf("connect: %+v %v", result, err)
			}
		}
		assertState := func(want string) {
			t.Helper()
			status, err := operations.Status(context.Background())
			if err != nil || status.State != want {
				t.Fatalf("status=%+v err=%v want=%s", status, err, want)
			}
			topology, err := operations.Topology(context.Background())
			if err != nil || topology.State != want {
				t.Fatalf("topology=%+v err=%v want=%s", topology, err, want)
			}
		}
		connect()
		old := <-observers
		release := <-releases
		assertState("CONNECTING")
		for _, test := range []struct {
			core agent.SupervisorState
			want string
		}{
			{agent.SupervisorActive, "CONNECTED"}, {agent.SupervisorBackoff, "RECONNECTING"},
			{agent.SupervisorAcquiring, "RECONNECTING"}, {agent.SupervisorActive, "CONNECTED"},
		} {
			old.ObserveSupervisor(agent.SupervisorSnapshot{State: test.core})
			assertState(test.want)
		}
		finished := make(chan localapi.OperationResult, 1)
		go func() {
			value, _ := operations.Disconnect(context.Background(), localapi.OperationRequest{})
			finished <- value
		}()
		synctest.Wait()
		assertState("DISCONNECTING")
		old.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorActive})
		assertState("DISCONNECTING")
		close(release)
		if result := <-finished; result.Code != "disconnected" {
			t.Fatalf("disconnect=%+v", result)
		}
		assertState("LOCAL")
		connect()
		current := <-observers
		currentRelease := <-releases
		old.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorActive})
		assertState("CONNECTING")
		current.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorActive})
		assertState("CONNECTED")
		if _, err := operations.SafeStop(context.Background()); err != nil {
			t.Fatal(err)
		}
		assertState("DISCONNECTING")
		close(currentRelease)
		synctest.Wait()
		assertState("LOCAL")
		if result, _ := operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"}); result.Code != "operation_cancelled" {
			t.Fatalf("restart after safe stop=%+v", result)
		}
	})
}

func TestUnexpectedRunnerErrorIsReportedWithoutRawDetails(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		operations := testOperations(t, func(context.Context, string, agent.SupervisorObserver) error {
			return errors.New("private address and internal path")
		})
		if result, _ := operations.Connect(context.Background(), localapi.OperationRequest{PeerAlias: "peer"}); result.Code != "connect_started" {
			t.Fatalf("connect=%+v", result)
		}
		synctest.Wait()
		diagnostics, err := operations.Diagnostics(context.Background())
		if err != nil || diagnostics.State != "session_failed" || diagnostics.Counters["session_failed"] != 1 {
			t.Fatalf("diagnostics=%+v err=%v", diagnostics, err)
		}
	})
}

func TestReconnectReasonCountersIgnoreStaleAndStoppingObservers(t *testing.T) {
	done := make(chan struct{})
	o := &Operations{root: context.Background(), sessionDone: done, sessionState: "CONNECTED", counters: map[string]uint64{}}
	observer := &sessionObserver{operations: o, done: done}
	snapshot := agent.SupervisorSnapshot{State: agent.SupervisorBackoff, FailureKind: "session", FailureComponent: "peer-reader", FailurePhase: "read", FailureCode: "after_local_interrupt", PeerTimeoutObserved: true}
	observer.ObserveSupervisor(snapshot)
	observer.ObserveSupervisor(agent.SupervisorSnapshot{State: agent.SupervisorAcquiring})
	if o.counters["reconnect_after_local_interrupt"] != 1 || o.counters["heartbeat_timeout_observed"] != 1 {
		t.Fatal("reason or independent timeout lost")
	}
	o.sessionDone = make(chan struct{})
	observer.ObserveSupervisor(snapshot)
	o.sessionDone = done
	o.sessionState = "DISCONNECTING"
	observer.ObserveSupervisor(snapshot)
	if o.counters["reconnect_after_local_interrupt"] != 1 || o.counters["heartbeat_timeout_observed"] != 1 {
		t.Fatal("stale or stopping observer recorded a retry")
	}
}
