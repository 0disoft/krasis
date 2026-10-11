package inputworker

import (
	"context"
	"errors"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/loginagent"
	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

// Fixed failure categories only. The authenticated child cannot send input,
// window names, endpoint details, a native message or a caller-selected reason.
type loginRuntimeFailure struct {
	reason string
	cause  error
}

func (e *loginRuntimeFailure) Error() string              { return "login runtime failed" }
func (e *loginRuntimeFailure) Unwrap() error              { return e.cause }
func (e *loginRuntimeFailure) LoginRuntimeReason() string { return e.reason }

func loginFailureKind(err error) loginControlKind {
	switch {
	case errors.Is(err, inputdesktop.ErrCleanup), errors.Is(err, ErrRecovery), errors.Is(err, loginagent.ErrRecovery):
		return loginFailedCleanup
	case errors.Is(err, inputdesktop.ErrInactive), errors.Is(err, inputdesktop.ErrUnavailable), errors.Is(err, inputdesktop.ErrWrongThread), errors.Is(err, inputdesktop.ErrClosed):
		return loginFailedDesktop
	case errors.Is(err, ErrAuthority), errors.Is(err, loginagent.ErrGrant), errors.Is(err, loginagent.ErrObservation), errors.Is(err, loginagent.ErrSuspended):
		return loginFailedPolicy
	case wininput.SourceFailureStage(err) != "":
		return loginFailedCapture
	case errors.Is(err, peer.ErrPeerLivenessExpired), errors.Is(err, peer.ErrPeerRemoteEOF), errors.Is(err, peer.ErrPeerReadFailure), errors.Is(err, peer.ErrPeerFrameTruncated):
		return loginFailedConnection
	}
	if _, known := application.InjectionCertaintyOf(err); known {
		return loginFailedInput
	}
	// Unknown failure remains unknown; cancellation without a confirmed clean
	// run is never upgraded to successful retirement.
	return loginFailed
}

func loginFailureReason(kind loginControlKind) string {
	switch kind {
	case loginFailedDesktop:
		return "desktop"
	case loginFailedPolicy:
		return "policy"
	case loginFailedInput:
		return "input"
	case loginFailedCapture:
		return "capture"
	case loginFailedCleanup:
		return "cleanup"
	case loginFailedConnection:
		return "connection"
	default:
		return "unknown"
	}
}

type observedLoginWorker struct {
	loginagent.Worker
	recorder *loginRuntimeRecorder
}

func (w *observedLoginWorker) Stop(ctx context.Context) error {
	err := w.Worker.Stop(ctx)
	state := "STOPPED"
	if err != nil {
		state = "FAILED"
	}
	w.recorder.record("worker", state, err)
	return err
}
