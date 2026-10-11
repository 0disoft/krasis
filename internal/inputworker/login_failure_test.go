package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/peer"
)

func TestLoginRuntimeFailureClassificationKeepsFixedReasons(t *testing.T) {
	for _, test := range []struct {
		err    error
		kind   loginControlKind
		reason string
	}{
		{inputdesktop.ErrInactive, loginFailedDesktop, "desktop"},
		{ErrAuthority, loginFailedPolicy, "policy"},
		{inputdesktop.ErrCleanup, loginFailedCleanup, "cleanup"},
		{peer.ErrPeerLivenessExpired, loginFailedConnection, "connection"},
		{errors.New("SECRET/input/path"), loginFailed, "unknown"},
	} {
		kind := loginFailureKind(test.err)
		if kind != test.kind || loginFailureReason(kind) != test.reason {
			t.Fatal("classification changed", kind)
		}
		if _, err := encodeLoginControl(loginTestBinding(t), kind); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLoginTerminalFailureRetainsReasonAndRecoveryRequirement(t *testing.T) {
	left, right := net.Pipe()
	defer left.Close()
	defer right.Close()
	binding := loginTestBinding(t)
	worker := startLoginControlWorker(context.Background(), left, binding, func() error { return nil })
	go func() {
		_ = writeLoginControl(right, binding, loginFailedDesktop)
		_, _, _ = readLoginControl(right)
	}()
	var saved logindiag.Runtime
	recorder := newLoginRuntimeRecorder(func(v logindiag.Runtime) error { saved = v; return nil })
	observed := &observedLoginWorker{Worker: worker, recorder: recorder}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	err := observed.Stop(ctx)
	if !errors.Is(err, ErrRecovery) || !saved.Valid() || saved.LastWorker != "FAILED" || saved.LastFailure == nil || saved.LastFailure.Stage != "worker" || saved.LastFailure.Reason != "desktop" {
		t.Fatalf("terminal evidence lost or treated clean: %v %+v", err, saved)
	}
	recorder.record("console", "USER_SESSION", nil)
	if saved.LastFailure.Reason != "desktop" {
		t.Fatal("login erased terminal failure")
	}
}
