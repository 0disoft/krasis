package wininput

import (
	"github.com/0disoft/krasis/internal/control"
	"testing"
)

func TestReturnBoundaryOscillationRequiresInwardRearm(t *testing.T) {
	for _, distance := range []int32{32, 64, 128} {
		adapter, err := newInputAdapterWithReturnRearmDistance(distance)
		if err != nil {
			t.Fatal(err)
		}
		intent := control.ReturnIntent{SessionID: "session", Epoch: "generation"}
		if err := adapter.ArmReturn(intent, control.ScreenEdgeRight); err != nil {
			t.Fatal(err)
		}
		observe := func(x int32) {
			t.Helper()
			if err := adapter.observeReturnPosition(x, 0, 1920); err != nil {
				t.Fatal(err)
			}
		}
		for i := 0; i < 100; i++ {
			observe(distance - 1)
			observe(0)
		}
		if len(adapter.ReturnIntents()) != 0 {
			t.Fatal("boundary jitter bypassed rearm")
		}
		observe(distance)
		observe(0)
		if len(adapter.ReturnIntents()) != 1 {
			t.Fatal("valid entry did not emit exactly once")
		}
		if got := <-adapter.ReturnIntents(); got != intent {
			t.Fatal("wrong generation")
		}
		for i := 0; i < 100; i++ {
			observe(distance)
			observe(0)
		}
		if len(adapter.ReturnIntents()) != 0 {
			t.Fatal("consumed generation emitted duplicate return")
		}
	}
}
