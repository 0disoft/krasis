package residentagent

import "testing"

func BenchmarkGCOSCounters(b *testing.B) {
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		processGCOSCounters()
	}
}

func TestGCOSIntervalSeparatesActualCPUFromRuntimeAccounting(t *testing.T) {
	p := gcOSCounters{cpu: 100, faults: 10, systemIdle: 100, systemKernel: 200, systemUser: 50, cpuKnown: true, faultsKnown: true, systemKnown: true}
	c := gcOSCounters{cpu: 300, faults: 13, systemIdle: 500, systemKernel: 1200, systemUser: 350, cpuKnown: true, faultsKnown: true, systemKnown: true}
	w := gcOSInterval(p, c)
	if !w.CPUKnown || w.CPUMicros != 200 || !w.FaultsKnown || w.PageFaults != 3 || !w.SystemKnown || w.SystemTotalMicros != 130 || w.SystemBusyMicros != 90 {
		t.Fatal("OS counters mixed or idle counted as busy", w)
	}
	w = gcOSInterval(p, p)
	if !w.CPUKnown || !w.FaultsKnown || !w.SystemKnown || w.CPUMicros != 0 || w.PageFaults != 0 || w.SystemTotalMicros != 0 {
		t.Fatal("measured zero became unknown", w)
	}
}

func TestGCOSIntervalUnknownRollbackAndOverflow(t *testing.T) {
	p := gcOSCounters{cpu: 10, faults: 20, systemIdle: 30, systemKernel: 40, systemUser: 50, cpuKnown: true, faultsKnown: true, systemKnown: true}
	for _, c := range []gcOSCounters{
		{},
		{cpu: 9, faults: 19, systemIdle: 29, systemKernel: 39, systemUser: 49, cpuKnown: true, faultsKnown: true, systemKnown: true},
		{systemIdle: 100, systemKernel: 41, systemUser: 50, systemKnown: true},
		{systemKernel: ^uint64(0), systemUser: 100, systemKnown: true},
	} {
		if w := gcOSInterval(p, c); w.CPUKnown || w.FaultsKnown || w.SystemKnown || w.CPUMicros != 0 || w.PageFaults != 0 || w.SystemBusyMicros != 0 || w.SystemTotalMicros != 0 {
			t.Fatal("unavailable/reset/overflow became data", w)
		}
	}
}
