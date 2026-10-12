package inputworker

import (
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

// Service/worker spans retain their own clock domains. Bootstrap contains child
// work; these observations must not be summed into a presentation duration.
func observePresentationStartup(emit func(string), profile *handoff.StartupProfile) {
	if emit == nil || profile == nil || !profile.Valid() {
		return
	}
	stages := [...]string{"service_launch", "service_child_connect", "service_bootstrap_ready", "worker_installed_validation", "worker_identity", "worker_service_auth", "worker_agent_auth", "worker_desktop_ready"}
	for stage, name := range stages {
		if profile.Measured&(1<<stage) != 0 {
			observePresentationPhase(emit, name, time.Duration(profile.Micros[stage])*time.Microsecond)
		}
	}
}

func observePresentationPhase(emit func(string), stage string, elapsed time.Duration) {
	if emit == nil {
		return
	}
	switch stage {
	case "pipe_connect", "service_launch", "service_child_connect", "service_bootstrap_ready", "worker_installed_validation", "worker_identity", "worker_service_auth", "worker_agent_auth", "worker_desktop_ready":
	default:
		return
	}
	bucket := timingBucket(elapsed)
	if bucket == 0 {
		return
	}
	suffix := [...]string{"", "lt1ms", "lt10ms", "lt100ms", "lt1s", "ge1s"}
	emitTiming(emit, "installed_return_cursor_startup_"+stage+"_"+suffix[bucket])
}
