package residentagent

import "github.com/0disoft/krasis/internal/productlog"

type gcOSCounters struct {
	readMicros                                        uint64
	cpu, faults, systemIdle, systemKernel, systemUser uint64
	cpuKnown, faultsKnown, systemKnown                bool
}

// Rollback/wrap and unavailable reads are unknown, never measured zero.
func gcOSInterval(p, c gcOSCounters) (w productlog.GCOSWindow) {
	w.ReadMicros = c.readMicros
	if p.cpuKnown && c.cpuKnown && c.cpu >= p.cpu {
		w.CPUMicros, w.CPUKnown = c.cpu-p.cpu, true
	}
	if p.faultsKnown && c.faultsKnown && c.faults >= p.faults {
		w.PageFaults, w.FaultsKnown = c.faults-p.faults, true
	}
	if p.systemKnown && c.systemKnown && c.systemIdle >= p.systemIdle && c.systemKernel >= p.systemKernel && c.systemUser >= p.systemUser {
		kernel, user, idle := c.systemKernel-p.systemKernel, c.systemUser-p.systemUser, c.systemIdle-p.systemIdle
		if kernel >= idle && user <= ^uint64(0)-kernel {
			w.SystemTotalMicros = (kernel + user) / 10
			w.SystemBusyMicros = (kernel - idle + user) / 10
			w.SystemKnown = true
		}
	}
	return
}
