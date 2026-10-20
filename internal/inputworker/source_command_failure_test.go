package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestFirstSourceCommandFailureSurvivesRetirementAndRuntimeReceipt(t *testing.T) {
	for op := sourceStart; op <= sourceCursorRestore; op++ {
		for _, cause := range []error{ErrAuthority, errors.New("private unknown failure")} {
			command := sourceCommandFailed(op, cause)
			// The loop subsequently stops or loses its reply. Neither replaces
			// the first command nor donates its sibling Win32 error to it.
			delivery := sourceTransportFailure(errors.Join(io.ErrClosedPipe, syscall.Errno(109)))
			final := sourceCaptureFinal(errors.Join(context.Canceled, command, delivery), inputdesktop.ErrInactive, context.Canceled, false)
			var wire bytes.Buffer
			if err := writeReport(&wire, Report{}, final); err != nil {
				t.Fatal(err)
			}
			_, received := readReport(&wire)
			recorder := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
			recorder.record("service", "FAILED", serviceEndpointFailure(received, "capture"))
			failure := recorder.value.LastFailure
			if failure == nil || failure.Operation != sourceCommandOperation(op) || failure.Win32Known || !recorder.value.Valid() {
				t.Fatal("first command lost or inherited reply error", op, failure)
			}
			uncertain := cause != ErrAuthority
			if errors.Is(received, ErrRecovery) != uncertain || (serviceRecoveryFailure(received, nil) != nil) != uncertain {
				t.Fatal("diagnostic marker changed cleanup certainty", op, received)
			}
		}
	}
}

func TestSourceCommandMarkerKeepsConcreteNativeAndAuthorityDetails(t *testing.T) {
	for _, specific := range []error{
		&workerReportFailure{operation: "cursor-restore", cause: errors.Join(ErrRecovery, syscall.Errno(170))},
		authorityCheckFailed("authority-console-token", syscall.Errno(5)),
	} {
		wantOp, wantNative := reportFailureDetails(specific)
		command := sourceCommandFailed(sourceActivate, errors.New("unknown private failure"))
		for _, failure := range []error{
			sourceCommandFailed(sourceActivate, specific),
			errors.Join(command, specific),
		} {
			op, native := reportFailureDetails(failure)
			if op != wantOp || native != wantNative {
				t.Fatal("command marker obscured concrete failure", reportOperations[op], native)
			}
		}
	}
	command := sourceCommandFailed(sourceDeactivate, syscall.Errno(170))
	op, native := reportFailureDetails(sourceCaptureFinal(command, inputdesktop.ErrInactive, context.Canceled, false))
	if reportOperations[op] != "source-command-deactivate" || native != 170 {
		t.Fatal("command lost its own native error", reportOperations[op], native)
	}
	if sourceCommandFailed(sourceActivate, nil) != nil {
		t.Fatal("successful command became a failure")
	}
}
