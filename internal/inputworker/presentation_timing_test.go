package inputworker

import (
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestPresentationStartupEmitsOnlyMeasuredValidatedPhases(t *testing.T) {
	var events []string
	emit := func(code string) { events = append(events, code) }
	var profile handoff.StartupProfile
	profile.Record(handoff.ServiceLaunch, 150*time.Millisecond)
	profile.Record(handoff.WorkerDesktopReady, 2*time.Millisecond)
	observePresentationStartup(emit, &profile)
	if len(events) != 2 || events[0] != "installed_return_cursor_startup_service_launch_lt1s" || events[1] != "installed_return_cursor_startup_worker_desktop_ready_lt10ms" {
		t.Fatal("phase presence or units changed", events)
	}
	profile.Micros[handoff.WorkerIdentity] = 1 // Unmeasured numbers are invalid.
	observePresentationStartup(emit, &profile)
	observePresentationPhase(emit, "private path", time.Millisecond)
	observePresentationPhase(emit, "pipe_connect", -time.Second)
	if len(events) != 2 {
		t.Fatal("invalid/unbounded phase data emitted", events)
	}
}
