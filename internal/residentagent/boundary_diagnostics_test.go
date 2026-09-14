package residentagent

import (
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/productlog"
	"sync"
	"testing"
)

func TestBoundaryDiagnosticsAggregateOnly(t *testing.T) {
	for _, edge := range []string{"LEFT", "RIGHT"} {
		d := &boundaryDiagnostics{summary: productlog.BoundarySummary{Left: -100, Width: 1920, Edge: edge}}
		var wg sync.WaitGroup
		for i := 0; i < 20; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				d.ObserveBoundaryIntent(control.BoundaryIntent{X: 1819})
				_ = d.snapshot(false)
			}()
		}
		wg.Wait()
		s := d.snapshot(true)
		want := int64(0)
		if edge == "LEFT" {
			want = 1919
		}
		if s.Candidates != 20 || s.LastOffset != want || !s.Complete {
			t.Fatalf("bad aggregate: %+v", s)
		}
		if d.snapshot(false).Complete {
			t.Fatal("snapshot mutated collection state")
		}
	}
}
