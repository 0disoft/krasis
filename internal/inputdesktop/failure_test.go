package inputdesktop

import (
	"errors"
	"syscall"
	"testing"
)

func TestDesktopFailurePreservesNativeCauseAndClosedOperation(t *testing.T) {
	for _, label := range []string{"desktop-bind", "desktop-restore", "desktop-close", "desktop-thread-exit"} {
		err := errors.Join(ErrCleanup, operationFailure(label, syscall.Errno(170)))
		if !errors.Is(err, ErrCleanup) || !errors.Is(err, syscall.Errno(170)) || FailureOperation(err) != label {
			t.Fatal("desktop failure metadata lost", err)
		}
	}
	if FailureOperation(operationFailure("PRIVATE/window/path", syscall.Errno(170))) != "" || operationFailure("desktop-bind", nil) != nil {
		t.Fatal("private data or success escaped operation boundary")
	}
}
