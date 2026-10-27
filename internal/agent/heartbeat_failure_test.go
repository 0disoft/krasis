package agent

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/failurecode"
	"github.com/0disoft/krasis/internal/peer"
)

func TestHeartbeatFailureRetainsBoundedCauseInExportedCode(t *testing.T) {
	for _, tc := range []struct {
		cause error
		code  string
	}{
		{application.ErrRuntimeNoLiveness, "no_liveness"},
		{control.ErrStaleHeartbeat, "stale"},
		{context.DeadlineExceeded, "deadline"},
		{context.Canceled, "canceled"},
		{errors.New("PRIVATE-peer-payload"), "failed"},
	} {
		err := errors.Join(peer.ErrPeerHeartbeatDispatch, tc.cause)
		d := DescribeSessionFailure(err)
		if d.Component != "peer-dispatch" || d.Phase != "heartbeat" || d.Code != tc.code {
			t.Fatal("heartbeat cause lost", d)
		}
		if code := failurecode.ReconnectCode("session", d.Component, d.Phase, d.Code); code != "reconnect_heartbeat_dispatch_"+tc.code {
			t.Fatal("classification not exportable", code)
		}
	}
}

func TestInputBacklogIsExportedWithoutLosingCauseBehindInjectionFailure(t *testing.T) {
	err := errors.Join(peer.ErrInputDispatchBacklog, peer.ErrPeerInputDispatch,
		&application.InjectionError{Certainty: application.InjectionOutcomeUnknown, Cause: context.Canceled})
	d := DescribeSessionFailure(err)
	if code := failurecode.ReconnectCode("session", d.Component, d.Phase, d.Code); code != "reconnect_input_queue_timeout" {
		t.Fatal("input backlog misclassified", d, code)
	}
}
