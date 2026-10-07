package productpanel

import (
	"context"
	"errors"
	"io"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

func TestPostExitHealthCheckContentionRestoresWithoutRepeatingInstallation(t *testing.T) {
	entered, finish := make(chan struct{}), make(chan struct{})
	refresh := &inputRefresh{inspect: func() string { close(entered); <-finish; return "READY" }, run: func(context.Context) error { t.Fatal("unexpected installation"); return nil }}
	refresh.mu.Lock()
	refresh.setState("READY")
	refresh.checkedAt = time.Now().Add(-time.Minute)
	refresh.mu.Unlock()
	refresh.status()
	<-entered
	defer func() { close(finish); refresh.mu.Lock(); refresh.mu.Unlock() }()
	c := &launchMaintenanceClient{inputMaintenanceClient: inputMaintenanceClient{fakeClient: fakeClient{state: "LOCAL"}}, generation: 4, build: "current"}
	starts := 0
	s := Startup{Generation: 4, Revision: "current", InputRefresh: refresh.explicit, InputSetup: refresh.status, BeforeStart: refresh.automatic,
		Lock: func() (io.Closer, error) { return closerStub{}, nil }, Verify: func() error { return nil },
		Open: func() (RuntimeClient, error) {
			if c.missing {
				return nil, localapi.ErrUnavailable
			}
			return c, nil
		},
		Available: func() (bool, error) { return c.missing, nil }, Pause: func(context.Context) error { return nil },
		Start: func() error { starts++; c.missing = false; return nil }}
	err := s.RefreshProtectedInput(context.Background())
	if !errors.Is(err, ErrStartupBusy) || !restoredInputAgent(err) || starts != 1 || c.missing || c.stops != 1 {
		t.Fatal("background reader left the agent stopped", err, starts, c.missing, c.stops)
	}
}

func TestOnlyVerifiedPostExitDeferralCanResumeThePausedConnection(t *testing.T) {
	for _, restored := range []bool{false, true} {
		b, c, _ := newResumeTest(t, nil)
		b.updatedResume = nil
		c.state = "CONNECTED"
		value := b.RefreshProtectedInput(func(context.Context) error {
			if restored {
				return &inputMaintenanceRestored{cause: ErrStartupBusy}
			}
			return ErrStartupBusy
		})
		want := 0
		if restored {
			want = 1
		}
		if value.InputRefreshCode != "busy" || value.agentPrepared != restored || c.connects != want {
			t.Fatal("unverified deferral resumed or verified recovery was lost", value, c.connects)
		}
	}
}
