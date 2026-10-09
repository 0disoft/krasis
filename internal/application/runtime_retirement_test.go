package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestPlannedRetirementReleasesHeldInputAndRejectsStaleLease(t *testing.T) {
	for _, fail := range []bool{false, true} {
		port := &recordingInputPort{}
		runtime := newTestRuntime(t, port)
		grantInbound(t, runtime, time.Unix(100, 0))
		identity := LeaseIdentity{SessionID: "session-1", LeaseID: "lease-1", Epoch: "epoch-1"}
		if err := runtime.HandleInputAction(context.Background(), control.InputEnvelope{SessionID: "session-1", Epoch: "epoch-1", Sequence: 1}, control.InputAction{Kind: control.InputKeyDown, Code: 1}); err != nil {
			t.Fatal(err)
		}
		stale := identity
		stale.LeaseID = "old"
		if _, _, err := runtime.RetireInbound(context.Background(), stale); !errors.Is(err, ErrStaleReturnIntent) || runtime.Snapshot().PressedCount != 1 {
			t.Fatal("stale retirement changed active lease", err)
		}
		if fail {
			port.releaseErr = errors.New("release blocked")
		}
		_, _, err := runtime.RetireInbound(context.Background(), identity)
		snapshot := runtime.Snapshot()
		if fail {
			if err == nil || snapshot.Control.State != control.StateFault || snapshot.PressedCount != 1 {
				t.Fatal("failed release escaped fault", snapshot, err)
			}
		} else if err != nil || snapshot.Control.State != control.StateLocal || snapshot.PressedCount != 0 {
			t.Fatal("planned return failed", snapshot, err)
		}
	}
}
