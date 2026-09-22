package agent

import (
	"context"
	"errors"
	"net"
	"sync"

	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/transport"
	"github.com/0disoft/krasis/internal/wininput"
)

var (
	ErrSourceSessionActivation = errors.New("source session activation failed")
	ErrSourceCaptureActivation = errors.New("source capture activation failed")
)

type FailureDescriptor struct {
	Component string
	Phase     string
	Code      string
}

func DescribeSessionFailure(err error) FailureDescriptor {
	switch {
	case errors.Is(err, wininput.ErrSourceCaptureUnregisterFailed):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-runtime", Code: "unregister_failed"}
	case errors.Is(err, wininput.ErrSourceCaptureCommandTimeout):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-runtime", Code: "command_timeout"}
	case errors.Is(err, wininput.ErrSourceCaptureLostForeground):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-runtime", Code: "foreground_lost"}
	case errors.Is(err, wininput.ErrSourceCaptureRollbackFailed):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "rollback_failed"}
	case errors.Is(err, wininput.ErrSourceFocusReleaseCaptureRetained):
		return FailureDescriptor{Component: "source-capture", Phase: "focus-release", Code: "capture_retained"}
	case errors.Is(err, wininput.ErrSourceFocusRestoreThreadDetachFailed):
		return FailureDescriptor{Component: "source-capture", Phase: "focus-restore", Code: "thread_detach_failed"}
	case errors.Is(err, wininput.ErrSourceFocusRestoreThreadAttachFailed):
		return FailureDescriptor{Component: "source-capture", Phase: "focus-restore", Code: "thread_attach_failed"}
	case errors.Is(err, wininput.ErrSourceFocusRestoreVerificationLost):
		return FailureDescriptor{Component: "source-capture", Phase: "focus-restore", Code: "verification_lost"}
	case errors.Is(err, wininput.ErrSourceFocusRestoreAPIRejected):
		return FailureDescriptor{Component: "source-capture", Phase: "focus-restore", Code: "api_rejected"}
	case errors.Is(err, wininput.ErrSourceCaptureThreadDetachFailed):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "thread_detach_failed"}
	case errors.Is(err, wininput.ErrSourceForegroundProbeFailed):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "foreground_probe_failed"}
	case errors.Is(err, wininput.ErrSourceForegroundThreadUnavailable):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "foreground_thread_unavailable"}
	case errors.Is(err, wininput.ErrSourceForegroundUnavailable):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "foreground_unavailable"}
	case errors.Is(err, wininput.ErrSourceCaptureThreadAttachFailed):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "thread_attach_failed"}
	case errors.Is(err, wininput.ErrSourceCaptureActivationVerificationLost):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "verification_lost"}
	case errors.Is(err, wininput.ErrSourceCaptureActivationAPIRejected):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "api_rejected"}
	case errors.Is(err, wininput.ErrSourceCaptureActivationSetup):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "setup_failed"}
	case errors.Is(err, ErrSourceCaptureActivation):
		return FailureDescriptor{Component: "source-capture", Phase: "capture-activate", Code: "activation_failed"}
	case errors.Is(err, ErrSourceSessionActivation):
		return FailureDescriptor{Component: "source-session", Phase: "source-activate", Code: "activation_failed"}
	}
	var write *transport.FrameWriteFailure
	if code := transport.FrameFailureCode(err); code != "" {
		return FailureDescriptor{Component: "peer-reader", Phase: "framing", Code: code}
	}
	if errors.As(err, &write) {
		code := "failed"
		var networkError net.Error
		if errors.Is(err, context.DeadlineExceeded) || errors.As(write.Cause, &networkError) && networkError.Timeout() {
			code = "timeout"
		} else if errors.Is(err, context.Canceled) {
			code = "canceled"
		}
		switch write.Phase {
		case "admission", "io", "deadline":
			return FailureDescriptor{Component: "peer-writer", Phase: "write-" + write.Phase, Code: code}
		}
	}
	switch {
	case errors.Is(err, peer.ErrSourceSessionInterrupted):
		return FailureDescriptor{Component: "source-session", Phase: "runtime", Code: "connection_interrupted"}
	case errors.Is(err, peer.ErrOutboundGrantApply):
		return FailureDescriptor{Component: "outbound-control", Phase: "grant-apply", Code: "state_transition_failed"}
	case errors.Is(err, peer.ErrOutboundBegin):
		return FailureDescriptor{Component: "boundary-admission", Phase: "request-begin", Code: "state_transition_failed"}
	case errors.Is(err, peer.ErrOutboundRequestWrite):
		return FailureDescriptor{Component: "peer-writer", Phase: "request-write", Code: "outcome_unknown"}
	case errors.Is(err, peer.ErrInboundGrantWrite):
		return FailureDescriptor{Component: "peer-writer", Phase: "grant-write", Code: "outcome_unknown"}
	case errors.Is(err, peer.ErrInboundRevokeWrite):
		return FailureDescriptor{Component: "peer-writer", Phase: "revoke-write", Code: "outcome_unknown"}
	case errors.Is(err, peer.ErrPeerReadAfterLocalInterrupt):
		return FailureDescriptor{Component: "peer-reader", Phase: "read", Code: "after_local_interrupt"}
	case errors.Is(err, peer.ErrPeerRemoteEOF):
		return FailureDescriptor{Component: "peer-reader", Phase: "read", Code: "remote_eof"}
	case errors.Is(err, peer.ErrPeerFrameTruncated):
		return FailureDescriptor{Component: "peer-reader", Phase: "read", Code: "frame_truncated"}
	case errors.Is(err, peer.ErrPeerReadFailure):
		return FailureDescriptor{Component: "peer-reader", Phase: "read", Code: "io_failure"}
	case errors.Is(err, peer.ErrPeerLivenessExpired):
		return FailureDescriptor{Component: "liveness", Phase: "heartbeat", Code: "peer_timeout"}
	case errors.Is(err, peer.ErrPeerControlDispatch):
		return FailureDescriptor{Component: "peer-dispatch", Phase: "control", Code: "failed"}
	case errors.Is(err, peer.ErrPeerInputDispatch):
		return FailureDescriptor{Component: "peer-dispatch", Phase: "input", Code: "failed"}
	case errors.Is(err, peer.ErrPeerHeartbeatDispatch):
		return FailureDescriptor{Component: "peer-dispatch", Phase: "heartbeat", Code: "failed"}
	}
	if stage := wininput.SourceFailureStage(err); stage != "" {
		return FailureDescriptor{Component: "source-hook", Phase: stage, Code: "failed"}
	}
	return FailureDescriptor{Component: SessionFailureComponent(err), Phase: "runtime", Code: "component_failed"}
}

type sessionConnectionEvidence struct {
	Connection
	mu         sync.Mutex
	localCause error
	observer   peer.ControlFlowObserver
}

func newSessionConnectionEvidence(connection Connection, observer peer.ControlFlowObserver) *sessionConnectionEvidence {
	return &sessionConnectionEvidence{Connection: connection, observer: observer}
}

func (connection *sessionConnectionEvidence) InterruptLocal(cause error) error {
	connection.mu.Lock()
	first := connection.localCause == nil
	if first {
		connection.localCause = cause
	}
	connection.mu.Unlock()
	if first {
		connection.observe(peer.ControlFlowLocalConnectionInterrupt)
	}
	return connection.Connection.Close()
}

func (connection *sessionConnectionEvidence) LocalInterruptObserved() bool {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.localCause != nil
}

func (connection *sessionConnectionEvidence) LocalInterruptCause() error {
	connection.mu.Lock()
	defer connection.mu.Unlock()
	return connection.localCause
}

func (connection *sessionConnectionEvidence) ObserveReadAfterLocalInterrupt() {
	connection.observe(peer.ControlFlowReadAfterLocalInterrupt)
}

func (connection *sessionConnectionEvidence) observe(event peer.ControlFlowEvent) {
	if connection != nil && connection.observer != nil {
		connection.observer.ObserveControlFlow(event)
	}
}
