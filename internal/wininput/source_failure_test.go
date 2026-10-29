package wininput

import (
	"context"
	"errors"
	"syscall"
	"testing"
)

func TestSourceFailureStagePreservesCauseAndDeepestOperation(t *testing.T) {
	cause := errors.New("private device details")
	err := sourceWorkerStage("raw-mouse", sourceStage("raw-buffer", sourceStage("raw-route", cause)))
	if !errors.Is(err, cause) || SourceFailureStage(err) != "raw-route" {
		t.Fatalf("cause or stage lost: %v / %s", err, SourceFailureStage(err))
	}
	if SourceFailureStage(sourceStage("private device details", cause)) != "" {
		t.Fatal("unknown stage exposed")
	}
	if sourceStage("raw-read", nil) != nil {
		t.Fatal("success changed")
	}
	err = errors.Join(sourceWorkerStage("low-level-hook", context.Canceled), err)
	if SourceFailureStage(err) != "raw-route" {
		t.Fatal("canceled sibling obscured failure")
	}
}

func TestSourceFailureDetailsKeepSelectedOperationNativeCause(t *testing.T) {
	for _, operation := range []string{"hook-release", "raw-release", "cursor-restore", "raw-window-destroy"} {
		err := errors.Join(syscall.Errno(5), sourceWorkerStage("raw-mouse", sourceStage(operation, syscall.Errno(170))))
		stage, cause := SourceFailureDetails(err)
		var native syscall.Errno
		if stage != operation || SourceFailureStage(err) != stage || !errors.As(cause, &native) || native != 170 {
			t.Fatal("joined sibling supplied another operation's native cause", stage, cause)
		}
	}
}

func TestHookProgressCannotHideConcreteRetirementFailure(t *testing.T) {
	for _, progress := range []string{"hook-callback-stalled", "hook-mouse-progress"} {
		trigger := sourceStage(progress, errHookCallbackStalled)
		for _, operation := range []string{"thread-join-multiple", "hook-release", "raw-release", "cursor-release", "raw-window-destroy"} {
			cleanup := sourceWorkerStage("low-level-hook", sourceStage(operation, syscall.Errno(170)))
			for _, failure := range []error{errors.Join(trigger, cleanup), errors.Join(cleanup, trigger), sourceWorkerStage("wheel-capture", errors.Join(trigger, context.Canceled, cleanup))} {
				stage, cause := SourceFailureDetails(failure)
				var native syscall.Errno
				if stage != operation || !errors.As(cause, &native) || native != 170 || !errors.Is(failure, errHookCallbackStalled) {
					t.Fatal("progress obscured native retirement or mixed sibling codes", stage, cause)
				}
			}
		}
		if stage, _ := SourceFailureDetails(errors.Join(trigger, context.Canceled, syscall.Errno(5))); stage != progress {
			t.Fatal("unclassified sibling replaced progress trigger", stage)
		}
	}
}
