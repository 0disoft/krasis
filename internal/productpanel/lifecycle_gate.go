package productpanel

import (
	"context"
	"sync"
)

// lifecycleGate keeps existing nonblocking UI operations while allowing a
// background handoff to wait in line without starving behind periodic reads.
// Cancellation does not spawn a waiter goroutine or leave an abandoned lock.
type lifecycleGate struct {
	once  sync.Once
	token chan struct{}
}

func (g *lifecycleGate) initialize() {
	g.once.Do(func() {
		g.token = make(chan struct{}, 1)
		g.token <- struct{}{}
	})
}

func (g *lifecycleGate) Lock() {
	g.initialize()
	<-g.token
}

func (g *lifecycleGate) TryLock() bool {
	g.initialize()
	select {
	case <-g.token:
		return true
	default:
		return false
	}
}

func (g *lifecycleGate) LockContext(ctx context.Context) bool {
	if ctx == nil || ctx.Err() != nil {
		return false
	}
	g.initialize()
	select {
	case <-ctx.Done():
		return false
	case <-g.token:
		if ctx.Err() != nil {
			g.Unlock()
			return false
		}
		return true
	}
}

func (g *lifecycleGate) Unlock() {
	g.initialize()
	select {
	case g.token <- struct{}{}:
	default:
		panic("unlock of unlocked lifecycle gate")
	}
}
