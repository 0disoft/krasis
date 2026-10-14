package inputworker

import (
	"bytes"
	"context"
	"errors"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestCaptureFinalKeepsEveryNativeAndSenderFailureFenced(t *testing.T) {
	if err := sourceCaptureFinal(context.Canceled, inputdesktop.ErrInactive, context.Canceled, false); err != nil {
		t.Fatal("joined desktop transition became a failure", err)
	}
	for _, tc := range []struct {
		name                  string
		loop, capture, sender error
		operation             string
	}{
		{"native cleanup", nil, inputdesktop.ErrCleanup, context.Canceled, "source-capture-join"},
		{"native join timeout", nil, context.DeadlineExceeded, context.Canceled, "source-capture-join"},
		{"sender join timeout", context.Canceled, context.Canceled, context.DeadlineExceeded, "source-sender-join"},
		{"command failure", ErrAuthority, context.Canceled, context.Canceled, "source-command-loop"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result := sourceCaptureFinal(tc.loop, tc.capture, tc.sender, false)
			// A failing final notification cannot erase a preceding cleanup fault.
			result = sourceRetirementDelivery(result, syscall.Errno(109))
			var wire bytes.Buffer
			if err := writeReport(&wire, Report{}, result); err != nil {
				t.Fatal(err)
			}
			report, received := readReport(&wire)
			_, received = receipt(report, received)
			failure := serviceRecoveryFailure(received, nil)
			var operation interface{ LoginRuntimeOperation() string }
			if !errors.Is(failure, ErrRecovery) || !errors.As(failure, &operation) || operation.LoginRuntimeOperation() != tc.operation {
				t.Fatal("cleanup failure or original classification lost", failure)
			}
		})
	}
}

func TestFinalFailureMetadataActuallyTraversesWriterAndReader(t *testing.T) {
	for _, label := range []string{"source-capture-join", "source-sender-join", "source-command-loop", "source-retirement-write", "source-channel-close"} {
		var wire bytes.Buffer
		failure := &workerReportFailure{operation: label, cause: errors.Join(ErrRecovery, syscall.Errno(170))}
		if err := writeReport(&wire, Report{}, failure); err != nil {
			t.Fatal(err)
		}
		_, received := readReport(&wire)
		var operation interface{ LoginRuntimeOperation() string }
		if !errors.As(received, &operation) || operation.LoginRuntimeOperation() != label || !errors.Is(received, syscall.Errno(170)) || !errors.Is(received, ErrRecovery) {
			t.Fatal("real report writer lost classified/native cause", label, received)
		}
	}
}
