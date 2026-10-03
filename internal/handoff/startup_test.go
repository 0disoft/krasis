package handoff

import (
	"testing"
	"time"
)

func TestStartupProfileBoundsPresenceAndFrozenAttempt(t *testing.T) {
	_, ctx, out := collected(t)
	p := Begin(ctx, "receiver", "session", "epoch", time.Now())
	Backend(p, "installed")
	var profile StartupProfile
	profile.Record(ServiceLaunch, Window+time.Second)
	profile.Record(WorkerIdentity, -time.Second)
	if !profile.Valid() || profile.Micros[ServiceLaunch] != uint32(Window.Microseconds()) || profile.Measured&(1<<WorkerIdentity) == 0 {
		t.Fatal("startup bounds or observed-zero presence lost")
	}
	RecordStartup(p, &profile)
	profile.Record(ServiceLaunch, time.Second)
	RecordStartup(p, &profile)
	Finish(p, "ok")
	RecordStartup(p, &profile)
	if len(*out) != 1 || (*out)[0].Startup == nil || (*out)[0].Startup.Micros[ServiceLaunch] != uint32(Window.Microseconds()) {
		t.Fatal("later mutation or duplicate receipt changed frozen startup")
	}
}

func TestStartupRejectsWrongRoleBackendExpiredAndUnmeasuredValues(t *testing.T) {
	for _, mode := range []string{"source", "native", "expired", "invalid"} {
		t.Run(mode, func(t *testing.T) {
			c, ctx, out := collected(t)
			role := "receiver"
			if mode == "source" {
				role = "source"
			}
			p := Begin(ctx, role, "session", "epoch", time.Now())
			if mode != "source" && mode != "native" {
				Backend(p, "installed")
			}
			var profile StartupProfile
			profile.Record(ServiceLaunch, time.Millisecond)
			if mode == "expired" {
				c.started = time.Now().Add(-Window)
			}
			if mode == "invalid" {
				profile.Micros[WorkerIdentity] = 1
			}
			RecordStartup(p, &profile)
			Finish(p, "ok")
			if len(*out) != 1 || (*out)[0].Startup != nil {
				t.Fatal("ineligible startup diagnostics attached")
			}
		})
	}
}
