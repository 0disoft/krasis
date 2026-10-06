package wininput

import (
	"testing"
	"time"
)

func TestPresentationTimeoutKeepsOnlyNewestStateAndClearsLateMark(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	applied := make(chan bool, 3)
	b := newSourcePresentationBroker(func(_ uintptr, enabled bool) bool {
		if enabled {
			close(entered)
			<-release
		}
		applied <- enabled
		return true
	})
	defer close(b.wake)
	s := &sourcePresentationState{window: 10}
	first := make(chan bool, 1)
	go func() { first <- b.request(s, true, 5*time.Millisecond) }()
	<-entered
	select {
	case accepted := <-first:
		if accepted {
			t.Fatal("stuck native call was acknowledged")
		}
	case <-time.After(time.Second):
		t.Fatal("native Shell call blocked the client past its deadline")
	}
	// No worker is created per timeout. The first clear is superseded by the
	// latest clear while the single native worker is still blocked.
	if b.request(s, false, time.Millisecond) {
		t.Fatal("blocked clear acknowledged")
	}
	last := make(chan bool, 1)
	go func() { last <- b.request(s, false, time.Second) }()
	deadline := time.After(time.Second)
	for {
		s.mu.Lock()
		queued := s.sequence == 3
		s.mu.Unlock()
		if queued {
			break
		}
		select {
		case <-deadline:
			t.Fatal("latest request not queued")
		default:
		}
	}
	close(release)
	if !<-last {
		t.Fatal("latest clear failed")
	}
	if !<-applied || <-applied {
		t.Fatal("late marking did not precede clearing")
	}
	select {
	case <-applied:
		t.Fatal("superseded request executed")
	default:
	}
}

func TestPresentationRetirementRejectsWorkForReusedWindowNumber(t *testing.T) {
	calls := make(chan uintptr, 1)
	b := newSourcePresentationBroker(func(window uintptr, _ bool) bool { calls <- window; return true })
	defer close(b.wake)
	old := &sourcePresentationState{window: 10}
	old.retire()
	if b.request(old, true, time.Second) {
		t.Fatal("retired window accepted")
	}
	current := &sourcePresentationState{window: 10}
	if !b.request(current, false, time.Second) || <-calls != 10 {
		t.Fatal("current window rejected")
	}
	select {
	case <-calls:
		t.Fatal("retired window request executed")
	default:
	}
}
