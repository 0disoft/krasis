package productpanel

import (
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestScheduledRecoveryStartsMissingAgentInExistingUI(t *testing.T) {
	missing := true
	starts, resumes := 0, 0
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) {
		if missing {
			return nil, localapi.ErrUnavailable
		}
		return &fakeClient{state: "LOCAL"}, nil
	}, func(context.Context) error { starts++; missing = false; return nil }, nil)
	resume := func(_ context.Context, v BridgeSnapshot) BridgeSnapshot { resumes++; return v }
	b.ScheduledRecover(func() bool { return false }, resume)
	if starts != 0 {
		t.Fatal("paused recovery started")
	}
	b.ScheduledRecover(func() bool { return true }, resume)
	if starts != 1 || resumes != 1 || b.Refresh().State != "LOCAL" {
		t.Fatal(starts, resumes, b.Refresh())
	}
	b.ScheduledRecover(func() bool { return true }, resume)
	if starts != 1 || resumes != 1 {
		t.Fatal("responding agent restarted")
	}
}
