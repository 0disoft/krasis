package agent

import (
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/peer"
	"github.com/0disoft/krasis/internal/wininput"
)

type agentControlFlowRecorder struct {
	events []peer.ControlFlowEvent
}

func (recorder *agentControlFlowRecorder) ObserveControlFlow(event peer.ControlFlowEvent) {
	recorder.events = append(recorder.events, event)
}

func (recorder *agentControlFlowRecorder) count(want peer.ControlFlowEvent) int {
	count := 0
	for _, event := range recorder.events {
		if event == want {
			count++
		}
	}
	return count
}

func TestCombinedSourceActivationClassifiesCaptureFailureStages(t *testing.T) {
	recorder := &agentControlFlowRecorder{}
	order := make([]string, 0, 4)
	source := &recordingActivation{name: "source", order: &order}
	capture := &recordingActivation{name: "capture", order: &order, activateErr: errors.New("foreground denied")}
	var observed error
	activation := &combinedSourceActivation{source: source, capture: capture, observer: recorder, failureObserver: func(err error) { observed = err }}
	err := activation.Activate()
	if observed != err {
		t.Fatalf("activation failure not reported: %v", observed)
	}
	if !errors.Is(err, ErrSourceCaptureActivation) || !errors.Is(err, capture.activateErr) {
		t.Fatalf("Activate() error = %v", err)
	}
	for _, want := range []peer.ControlFlowEvent{
		peer.ControlFlowSourceSessionActivationSucceeded,
		peer.ControlFlowCaptureActivationAttempted,
		peer.ControlFlowCaptureActivationFailed,
		peer.ControlFlowCaptureActivationSetupFailed,
	} {
		if recorder.count(want) != 1 {
			t.Fatalf("event %d count=%d events=%v", want, recorder.count(want), recorder.events)
		}
	}
	if recorder.count(peer.ControlFlowCaptureActivationSucceeded) != 0 {
		t.Fatalf("capture success recorded after failure: %v", recorder.events)
	}
}

func TestCombinedSourceActivationPreservesForegroundFailureSubtype(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
		want peer.ControlFlowEvent
	}{
		{name: "API rejected", err: wininput.ErrSourceCaptureActivationAPIRejected, want: peer.ControlFlowCaptureActivationAPIRejected},
		{name: "verification lost", err: wininput.ErrSourceCaptureActivationVerificationLost, want: peer.ControlFlowCaptureActivationVerificationLost},
	} {
		t.Run(test.name, func(t *testing.T) {
			recorder := &agentControlFlowRecorder{}
			order := make([]string, 0, 4)
			activation := &combinedSourceActivation{
				source:   &recordingActivation{name: "source", order: &order},
				capture:  &recordingActivation{name: "capture", order: &order, activateErr: test.err},
				observer: recorder,
			}
			if err := activation.Activate(); !errors.Is(err, test.err) {
				t.Fatalf("Activate() error = %v", err)
			}
			if recorder.count(test.want) != 1 || recorder.count(peer.ControlFlowCaptureActivationSetupFailed) != 0 {
				t.Fatalf("events = %v", recorder.events)
			}
		})
	}
}

func TestDescribeSessionFailurePrefersPreservedSourceRoot(t *testing.T) {
	err := errors.Join(
		peer.ErrPeerReadAfterLocalInterrupt,
		peer.ErrSourceSessionInterrupted,
		errors.New("private transport details"),
	)
	descriptor := DescribeSessionFailure(err)
	if descriptor.Component != "source-session" || descriptor.Phase != "runtime" || descriptor.Code != "connection_interrupted" {
		t.Fatalf("descriptor = %+v", descriptor)
	}

	descriptor = DescribeSessionFailure(errors.Join(ErrSourceCaptureActivation, peer.ErrPeerReadFailure))
	if descriptor.Component != "source-capture" || descriptor.Phase != "capture-activate" || descriptor.Code != "activation_failed" {
		t.Fatalf("capture descriptor = %+v", descriptor)
	}
}

func TestDescribeSessionFailureClassifiesCaptureActivationCause(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "setup", err: wininput.ErrSourceCaptureActivationSetup, code: "setup_failed"},
		{name: "rollback", err: wininput.ErrSourceCaptureRollbackFailed, code: "rollback_failed"},
		{name: "no foreground", err: errors.Join(wininput.ErrSourceCaptureThreadAttachFailed, wininput.ErrSourceForegroundUnavailable), code: "foreground_unavailable"},
		{name: "no thread", err: errors.Join(wininput.ErrSourceCaptureThreadAttachFailed, wininput.ErrSourceForegroundThreadUnavailable), code: "foreground_thread_unavailable"},
		{name: "probe", err: errors.Join(wininput.ErrSourceCaptureThreadAttachFailed, wininput.ErrSourceForegroundProbeFailed), code: "foreground_probe_failed"},
		{name: "API rejected", err: wininput.ErrSourceCaptureActivationAPIRejected, code: "api_rejected"},
		{name: "verification lost", err: wininput.ErrSourceCaptureActivationVerificationLost, code: "verification_lost"},
		{name: "thread attach", err: errors.Join(wininput.ErrSourceCaptureActivationAPIRejected, wininput.ErrSourceCaptureThreadAttachFailed), code: "thread_attach_failed"},
		{name: "thread detach", err: errors.Join(wininput.ErrSourceCaptureActivationAPIRejected, wininput.ErrSourceCaptureThreadDetachFailed), code: "thread_detach_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := DescribeSessionFailure(test.err)
			if descriptor.Component != "source-capture" || descriptor.Phase != "capture-activate" || descriptor.Code != test.code {
				t.Fatalf("descriptor = %+v", descriptor)
			}
		})
	}
}

func TestDescribeCaptureRuntimeFailure(t *testing.T) {
	for cause, code := range map[error]string{
		wininput.ErrSourceCaptureUnregisterFailed: "unregister_failed",
		wininput.ErrSourceCaptureCommandTimeout:   "command_timeout",
		wininput.ErrSourceCaptureLostForeground:   "foreground_lost",
	} {
		d := DescribeSessionFailure(errors.Join(cause, errors.New("private native details")))
		if d.Component != "source-capture" || d.Phase != "capture-runtime" || d.Code != code {
			t.Fatalf("descriptor = %+v", d)
		}
	}
}

func TestDescribeSessionFailureClassifiesFocusRestoreCause(t *testing.T) {
	tests := []struct {
		name string
		err  error
		code string
	}{
		{name: "API rejected", err: wininput.ErrSourceFocusRestoreAPIRejected, code: "api_rejected"},
		{name: "verification lost", err: wininput.ErrSourceFocusRestoreVerificationLost, code: "verification_lost"},
		{name: "thread attach", err: errors.Join(wininput.ErrSourceFocusRestoreAPIRejected, wininput.ErrSourceFocusRestoreThreadAttachFailed), code: "thread_attach_failed"},
		{name: "thread detach", err: errors.Join(wininput.ErrSourceFocusRestoreAPIRejected, wininput.ErrSourceFocusRestoreThreadDetachFailed), code: "thread_detach_failed"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := DescribeSessionFailure(test.err)
			if descriptor.Component != "source-capture" || descriptor.Phase != "focus-restore" || descriptor.Code != test.code {
				t.Fatalf("descriptor = %+v", descriptor)
			}
		})
	}
}

func TestDescribeSessionFailureClassifiesRetainedCaptureFocus(t *testing.T) {
	descriptor := DescribeSessionFailure(errors.Join(
		wininput.ErrSourceFocusRestoreAPIRejected,
		wininput.ErrSourceFocusReleaseCaptureRetained,
	))
	if descriptor.Component != "source-capture" || descriptor.Phase != "focus-release" || descriptor.Code != "capture_retained" {
		t.Fatalf("descriptor = %+v", descriptor)
	}
}

func TestDescribeSessionFailureClassifiesPeerDispatchStage(t *testing.T) {
	tests := []struct {
		name  string
		err   error
		phase string
	}{
		{name: "control", err: peer.ErrPeerControlDispatch, phase: "control"},
		{name: "input", err: peer.ErrPeerInputDispatch, phase: "input"},
		{name: "heartbeat", err: peer.ErrPeerHeartbeatDispatch, phase: "heartbeat"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			descriptor := DescribeSessionFailure(errors.Join(test.err, errors.New("private detail")))
			if descriptor.Component != "peer-dispatch" || descriptor.Phase != test.phase || descriptor.Code != "failed" {
				t.Fatalf("descriptor = %+v", descriptor)
			}
		})
	}
}
