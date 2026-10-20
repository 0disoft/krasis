package netobserve

import (
	"context"
	"runtime"
	"testing"
	"time"
)

func TestStalledNativeObservationCannotBlockCancellationOrMultiplyOwners(t *testing.T) {
	g := &sampleGate{}
	entered, release := make(chan struct{}), make(chan struct{})
	ctx, cancel := context.WithCancel(context.Background())
	result := make(chan Snapshot, 1)
	go func() {
		result <- g.read(ctx, func() Snapshot { close(entered); <-release; return Snapshot{Power: PowerAC} })
	}()
	<-entered
	cancel()
	select {
	case value := <-result:
		if value.Power != PowerUnknown {
			t.Fatal("cancelled observation published")
		}
	case <-time.After(time.Second):
		t.Fatal("native query blocked cancellation")
	}
	for i := 0; i < 10; i++ {
		if value := g.read(context.Background(), func() Snapshot { t.Fatal("another native query started while old owner was stuck"); return Snapshot{} }); value.Power != PowerUnknown {
			t.Fatal("old probe result reused")
		}
	}
	close(release)
	deadline := time.Now().Add(time.Second)
	for g.busy.Load() {
		if time.Now().After(deadline) {
			t.Fatal("probe owner did not finish")
		}
		runtime.Gosched()
	}
	if value := g.read(context.Background(), func() Snapshot { return Snapshot{Power: PowerBattery} }); value.Power != PowerBattery {
		t.Fatal("late old result replaced fresh owner")
	}
}
