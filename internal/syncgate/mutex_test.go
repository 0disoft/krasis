package syncgate

import (
	"context"
	"errors"
	"testing"
	"time"
)

func TestExpiredWaitersDoNotTakeOrReleaseOwnersPermit(t *testing.T) {
	var m Mutex
	m.Lock()
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	done := make(chan error, 16)
	for i := 0; i < cap(done); i++ {
		go func() { done <- m.LockContext(ctx) }()
	}
	for i := 0; i < cap(done); i++ {
		select {
		case err := <-done:
			if !errors.Is(err, context.DeadlineExceeded) {
				t.Fatal(err)
			}
		case <-time.After(time.Second):
			t.Fatal("lock wait ignored cancellation")
		}
	}
	if m.TryLock() {
		t.Fatal("canceled waiter released the owner's permit")
	}
	m.Unlock()
	if err := m.LockContext(ctx); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("expired waiter admitted")
	}
	if !m.TryLock() {
		t.Fatal("canceled waiters retained a permit")
	}
	m.Unlock()
}
