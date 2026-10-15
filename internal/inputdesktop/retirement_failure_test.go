package inputdesktop

import (
	"errors"
	"strings"
	"syscall"
	"testing"
)

func TestCaptureThreadFailureSeparatesStageStartupAndWaitWithoutLosingCause(t *testing.T) {
	for _, stage := range []string{"pin", "wait", "close"} {
		outcomes := []string{""}
		if stage == "wait" {
			outcomes = []string{"timeout", "failed", "unexpected"}
		}
		if stage == "close" {
			outcomes = []string{"signaled", "timeout", "failed", "unexpected"}
		}
		for _, startup := range []bool{false, true} {
			owner := "other"
			if startup {
				owner = "startup"
			}
			for _, outcome := range outcomes {
				want := "desktop-thread-" + stage + "-" + owner
				if outcome != "" {
					want += "-" + outcome
				}
				cause := syscall.Errno(170)
				err := operationFailure("desktop-thread-exit", captureThreadFailure(stage, startup, outcome, cause))
				if FailureOperation(err) != want || !errors.Is(err, cause) {
					t.Fatal("native failure classification lost", want, err)
				}
			}
		}
	}
	if captureThreadFailure("close", false, "signaled", nil) != nil {
		t.Fatal("successful close became a failure")
	}
	for _, private := range []string{"PRIVATE/path", "12345", "signaled"} {
		err := captureThreadFailure("wait", false, private, errors.New("PRIVATE/error"))
		if FailureOperation(err) != "desktop-thread-exit" || strings.Contains(FailureOperation(err), private) {
			t.Fatal("unbounded or impossible wait classification accepted")
		}
	}
	copy := CaptureThreadFailureOperations()
	copy[0] = "PRIVATE/path"
	if ValidCaptureThreadFailureOperation(copy[0]) || !ValidCaptureThreadFailureOperation("desktop-thread-pin-other") {
		t.Fatal("caller mutated the closed operation vocabulary")
	}
}
