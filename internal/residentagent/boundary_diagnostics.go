package residentagent

import (
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/productlog"
	"sync"
)

type boundaryDiagnostics struct {
	mu      sync.Mutex
	summary productlog.BoundarySummary
}

func (*boundaryDiagnostics) ObserveSourceAction(control.InputAction) {}

func (d *boundaryDiagnostics) ObserveBoundaryIntent(intent control.BoundaryIntent) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.summary.Candidates != ^uint64(0) {
		d.summary.Candidates++
	}
	edge := int64(d.summary.Left)
	if d.summary.Edge == "RIGHT" {
		edge += int64(d.summary.Width) - 1
	}
	d.summary.LastOffset = int64(intent.X) - edge
}

func (d *boundaryDiagnostics) snapshot(complete bool) productlog.BoundarySummary {
	d.mu.Lock()
	defer d.mu.Unlock()
	s := d.summary
	s.Complete = complete
	return s
}
