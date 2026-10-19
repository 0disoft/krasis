package inputdesktop

import (
	"errors"
	"syscall"
	"testing"
)

func TestDesktopFailurePreservesNativeCauseAndClosedOperation(t *testing.T) {
	for _, label := range []string{"desktop-bind", "desktop-restore", "desktop-close", "desktop-thread-exit"} {
		err := errors.Join(syscall.Errno(5), ErrCleanup, operationFailure(label, syscall.Errno(170)))
		if !errors.Is(err, ErrCleanup) || !errors.Is(err, syscall.Errno(170)) || FailureOperation(err) != label {
			t.Fatal("desktop failure metadata lost", err)
		}
		operation, cause := FailureDetails(err)
		var native syscall.Errno
		if operation != label || !errors.As(cause, &native) || native != 170 {
			t.Fatal("selected desktop operation used sibling native cause", operation, cause)
		}
	}
	if FailureOperation(operationFailure("PRIVATE/window/path", syscall.Errno(170))) != "" || operationFailure("desktop-bind", nil) != nil {
		t.Fatal("private data or success escaped operation boundary")
	}
}
