package x11registry

import (
	"sync"
	"sync/atomic"
	"testing"
)

func TestConcurrentSameServerRegistersDecodersOnce(t *testing.T) {
	var r registry
	var writes atomic.Int32
	var workers sync.WaitGroup
	for range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := r.record(90, 140, func() { writes.Add(1) }); err != nil {
				t.Error(err)
			}
		}()
	}
	workers.Wait()
	if writes.Load() != 1 {
		t.Fatal("global decoders registered more than once")
	}
	for _, bases := range [][2]byte{{91, 140}, {90, 141}} {
		if err := r.record(bases[0], bases[1], func() { writes.Add(1) }); err == nil {
			t.Fatal("different server bases accepted")
		}
	}
	if writes.Load() != 1 {
		t.Fatal("rejected server rewrote decoders")
	}
}
