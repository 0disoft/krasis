// Package syncgate provides cancellation-aware serialization without polling
// or detached lock waiters. A mutex must not be copied after first use.
package syncgate

import (
	"context"
	"sync"
)

// Mutex has a usable zero value. Ordinary lifecycle calls can still join the
// owner using Lock; input admission uses LockContext to respect its budget.
type Mutex struct {
	once sync.Once
	gate chan struct{}
}

func (m *Mutex) init() { m.once.Do(func() { m.gate = make(chan struct{}, 1) }) }

func (m *Mutex) Lock() { m.init(); m.gate <- struct{}{} }
func (m *Mutex) Unlock() {
	select {
	case <-m.gate:
	default:
		panic("syncgate: unlock of unlocked mutex")
	}
}
func (m *Mutex) TryLock() bool {
	m.init()
	select {
	case m.gate <- struct{}{}:
		return true
	default:
		return false
	}
}

func (m *Mutex) LockContext(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	m.init()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case m.gate <- struct{}{}:
		// Cancellation may race the unlock. Never admit work with an already
		// expired context, or retain a permit on behalf of a departed waiter.
		if err := ctx.Err(); err != nil {
			m.Unlock()
			return err
		}
		return nil
	}
}
