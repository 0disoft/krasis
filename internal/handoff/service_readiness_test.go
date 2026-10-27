package handoff

import (
	"testing"
	"time"
)

func TestServiceReadinessBoundsPresenceAndFrozenAttempt(t *testing.T) {
	_, ctx, out := collected(t)
	attempt := Begin(ctx, "receiver", "session", "epoch", time.Now())
	Backend(attempt, "installed")
	var p StartupProfile
	p.Record(ServiceBootstrapReady, time.Millisecond)
	p.ServiceReadiness.Record(ServiceLoginAdmission, Window+time.Second)
	p.ServiceReadiness.Record(ServiceReadyAuthorization, -time.Second)
	if !p.Valid() || p.ServiceReadiness.Measured != 18 || p.ServiceReadiness.Micros[ServiceLoginAdmission] != uint32(Window.Microseconds()) {
		t.Fatal("invalid bounds or lost measured zero", p)
	}
	RecordStartup(attempt, &p)
	p.ServiceReadiness.Record(ServiceLoginAdmission, 0)
	Finish(attempt, "ok")
	if len(*out) != 1 || (*out)[0].Startup.ServiceReadiness.Micros[ServiceLoginAdmission] != uint32(Window.Microseconds()) {
		t.Fatal("service diagnostics did not freeze by value")
	}
	for _, invalid := range []ServiceReadinessProfile{
		{Measured: 1 << ServiceReadinessStageCount},
		{Micros: [ServiceReadinessStageCount]uint32{1}},
		{Measured: 1, Micros: [ServiceReadinessStageCount]uint32{uint32(Window.Microseconds() + 1)}},
	} {
		p.ServiceReadiness = invalid
		if p.Valid() {
			t.Fatal("invalid service readiness accepted", invalid)
		}
	}
}
