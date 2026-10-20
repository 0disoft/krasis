package netobserve

import (
	"context"
	"sync/atomic"
	"time"
)

// Native WLAN/COM queries do not accept cancellation. One process-wide probe
// may finish late, but it cannot retain a connection/monitor or spawn another
// native query while stuck. Its private reply can never reach a later owner.
type sampleGate struct{ busy atomic.Bool }

var platformProbe sampleGate

func (g *sampleGate) read(ctx context.Context, sample func() Snapshot) Snapshot {
	if ctx.Err() != nil || !g.busy.CompareAndSwap(false, true) {
		return Snapshot{}
	}
	result := make(chan Snapshot, 1)
	go func() {
		value := Snapshot{}
		defer func() { _ = recover(); g.busy.Store(false); result <- value }()
		value = sample()
	}()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case value := <-result:
		if ctx.Err() == nil {
			return value
		}
	case <-ctx.Done():
	case <-timer.C:
	}
	return Snapshot{}
}
