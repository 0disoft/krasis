package wininput

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func activatedOfferSession(t *testing.T) *SourceSession {
	t.Helper()
	s, err := NewSourceSession(8, &recordingSourceSink{})
	if err != nil {
		t.Fatal(err)
	}
	s.started.Store(true)
	if err := s.Activate(); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestPhysicalDiscreteAndWheelOfferDoNotWaitForStateTransaction(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := activatedOfferSession(t)
		s.stateMu.Lock()
		defer s.stateMu.Unlock()
		done := make(chan bool, 1)
		go func() {
			accepted := true
			for _, action := range []control.InputAction{
				{Kind: control.InputKeyDown, Code: 30},
				{Kind: control.InputKeyUp, Code: 30},
				{Kind: control.InputButtonDown, Code: MouseButtonLeft},
				{Kind: control.InputButtonUp, Code: MouseButtonLeft},
				{Kind: control.InputWheel, DeltaY: 120},
			} {
				accepted = routePhysicalAction(s, nil, action, 0) && accepted
			}
			done <- accepted
		}()
		synctest.Wait()
		select {
		case accepted := <-done:
			if !accepted || len(s.queue) != 5 || s.terminal.Load() || s.PhysicalPressCount() != 0 {
				t.Fatal("state contention lost physical ordering or forced recovery")
			}
		default:
			t.Fatal("physical callback waited for the state transaction")
		}
		current := make(chan bool, 1)
		go func() { current <- s.current(s.generation.Load()) }()
		synctest.Wait()
		select {
		case valid := <-current:
			if !valid {
				t.Fatal("active sender generation became invalid")
			}
		default:
			t.Fatal("sender semantic write check waited for the state transaction")
		}
	})
}

func TestPausedOfferCannotCrossLeaseGeneration(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s := activatedOfferSession(t)
		old := s.generation.Load()
		captured, release := make(chan struct{}), make(chan struct{})
		s.now = func() time.Time { close(captured); <-release; return time.Now() }
		done := make(chan bool, 1)
		go func() { done <- s.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) }()
		<-captured // Pause after generation capture, before queue admission.
		s.Deactivate()
		if err := s.Activate(); err != nil {
			close(release)
			t.Fatal(err)
		}
		close(release)
		if <-done || len(s.queue) != 0 || s.current(old) || !s.Active() || s.terminal.Load() {
			t.Fatal("old queue producer entered a new lease or forced its recovery")
		}
		s.now = time.Now
		if !s.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 2}) {
			t.Fatal("fresh action could not enter the new lease")
		}
	})
}

func TestLocalReturnPublishesGenerationBeforeCompletion(t *testing.T) {
	s := activatedOfferSession(t)
	old := s.generation.Load()
	if !s.RequestLocalReturn() || s.current(old) || s.Offer(control.InputAction{Kind: control.InputKeyUp, Code: 30}) {
		t.Fatal("local return left old actions eligible")
	}
	if err := s.Activate(); err == nil {
		t.Fatal("pending local cleanup admitted another lease")
	}
	s.CompleteLocalReturn(<-s.LocalReturns())
	if err := s.Activate(); err != nil {
		t.Fatal(err)
	}
	if s.current(old) || !s.current(s.generation.Load()) {
		t.Fatal("return completion revived an old generation")
	}
}
