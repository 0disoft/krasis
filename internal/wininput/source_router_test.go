package wininput

import (
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestRoutePhysicalActionObservesBoundaryWhileSourceInactive(t *testing.T) {
	session := newInactiveSourceSession(t)
	observer := &recordingBoundaryObserver{}
	action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 3}
	if routePhysicalAction(session, observer, action, 99) {
		t.Fatal("inactive source suppressed local input")
	}
	if observer.calls != 1 || observer.pointerX != 99 || observer.action != action {
		t.Fatalf("observer = %+v", observer)
	}
}

func TestRoutePhysicalActionKeepsLocalInputOnFullBoundaryQueue(t *testing.T) {
	session := newInactiveSourceSession(t)
	observer := &recordingBoundaryObserver{err: control.ErrBoundaryIntentQueueFull}
	if routePhysicalAction(session, observer, control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}, 99) {
		t.Fatal("queue overflow suppressed local input")
	}
}

func TestRoutePhysicalActionAbortsActiveSourceOnBoundaryFailure(t *testing.T) {
	sink := &recordingSourceSink{failed: make(chan error, 1)}
	session := startSourceSession(t, 1, sink)
	cause := errors.New("boundary state corrupt")
	if routePhysicalAction(session, &recordingBoundaryObserver{err: cause}, control.InputAction{Kind: control.InputKeyDown, Code: 1}, 0) {
		t.Fatal("failed boundary observation suppressed local input")
	}
	select {
	case err := <-sink.failed:
		if !errors.Is(err, cause) {
			t.Fatalf("Fail() cause = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("active source did not report boundary failure")
	}
}

func TestRoutePhysicalActionFailsOpenWhenBoundaryObserverPanics(t *testing.T) {
	sink := &recordingSourceSink{failed: make(chan error, 1)}
	session := startSourceSession(t, 1, sink)
	if routePhysicalAction(session, panickingBoundaryObserver{"private input"}, control.InputAction{Kind: control.InputKeyDown, Code: 1}, 0) {
		t.Fatal("panicking boundary observer suppressed local input")
	}
	select {
	case err := <-sink.failed:
		if !errors.Is(err, ErrBoundaryObserverPanic) {
			t.Fatalf("Fail() cause = %v", err)
		}
		if err.Error() != ErrBoundaryObserverPanic.Error() {
			t.Fatalf("panic payload leaked through failure: %q", err)
		}
	case <-time.After(time.Second):
		t.Fatal("source did not report boundary observer panic")
	}
	if session.Active() {
		t.Fatal("boundary observer panic left suppression active")
	}
}

func newInactiveSourceSession(t *testing.T) *SourceSession {
	t.Helper()
	session, err := NewSourceSession(1, &recordingSourceSink{})
	if err != nil {
		t.Fatalf("NewSourceSession() error = %v", err)
	}
	return session
}

type recordingBoundaryObserver struct {
	calls    int
	action   control.InputAction
	pointerX int32
	err      error
}

func (observer *recordingBoundaryObserver) Observe(action control.InputAction, pointerX int32) error {
	observer.calls++
	observer.action = action
	observer.pointerX = pointerX
	return observer.err
}

type panickingBoundaryObserver struct {
	value any
}

func (observer panickingBoundaryObserver) Observe(control.InputAction, int32) error {
	panic(observer.value)
}
