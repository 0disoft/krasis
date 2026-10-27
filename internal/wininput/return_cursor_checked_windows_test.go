//go:build windows

package wininput

import (
	"errors"
	"testing"
)

func TestOptionalCursorSnapshotDoesNotHideNativeCleanupFailure(t *testing.T) {
	hook := &SourceHook{}
	if prepared, err := hook.PrepareReturnCursorChecked(); prepared != nil || err != nil {
		t.Fatal("missing shield became a native failure", err)
	}
	cause := errors.New("unconfirmed cursor cleanup")
	hook.returnCursor.record("cursor-restore", cause)
	if prepared, err := hook.PrepareReturnCursorChecked(); prepared != nil || !errors.Is(err, cause) {
		t.Fatal("optional snapshot erased native cleanup failure", err)
	}
}
