package productpanel

import (
	"context"
	"errors"
	"testing"
)

func TestVerifiedStartupObservationClearsOnlyPreparationNoticesOnce(t *testing.T) {
	for _, code := range []string{"startup_busy", "startup_failed", "agent_busy", "agent_incompatible", "startup_resume_settings_invalid", "diagnostic_failed"} {
		t.Run(code, func(t *testing.T) {
			b, c, now := newResumeTest(t, func(context.Context) error { t.Fatal("observation restarted agent"); return nil })
			b.updatedResume = nil
			ready, promotions := false, 0
			b.StartupObservedReady = func(context.Context) bool { return ready }
			b.StartupReady = func() { promotions++ }
			b.save(BridgeSnapshot{State: "LOCAL", Code: code, InputRefreshCode: "input_refresh_cancelled", ReconnectCode: "not_attempted"})
			b.tick(now)
			if b.Refresh().Code != code || promotions != 0 {
				t.Fatal("unverified failure cleared", b.Refresh())
			}
			ready = true
			b.tick(now)
			b.tick(now)
			want, wantPromotions := "ok", 1
			if code == "startup_resume_settings_invalid" || code == "diagnostic_failed" {
				want, wantPromotions = code, 0
			}
			if b.Refresh().Code != want || promotions != wantPromotions || c.connects != 0 || b.Refresh().InputRefreshCode != "input_refresh_cancelled" {
				t.Fatal("unrelated failure erased, readiness replayed or reconnect sent", b.Refresh(), promotions, c.connects)
			}
		})
	}
}

func TestStartupObservationRequiresMatchingHealthyBuildAndCompleteBundle(t *testing.T) {
	for _, mode := range []string{"healthy", "wrong build", "old build", "faulted", "unreadable", "broken bundle", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			c := &launchMaintenanceClient{inputMaintenanceClient: inputMaintenanceClient{fakeClient: fakeClient{state: "LOCAL"}}, generation: 5, build: "current"}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			s := Startup{Generation: c.generation, Revision: c.build, Open: func() (RuntimeClient, error) { return c, nil },
				Verify: func() error {
					if mode == "broken bundle" {
						return errors.New("bad bundle")
					}
					return nil
				},
				Start: func() error { t.Fatal("started on observation"); return nil }}
			switch mode {
			case "wrong build":
				c.build = "different"
			case "old build":
				c.generation--
			case "faulted":
				c.state = "FAULTED"
			case "unreadable":
				c.err = errors.New("unavailable")
			case "cancelled":
				cancel()
			}
			if s.ObservedReady(ctx) != (mode == "healthy") {
				t.Fatal("unverified readiness", mode)
			}
		})
	}
}
