package wininput

import (
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestRouteMappedPhysicalActionFailsOpenWhenMapperPanics(t *testing.T) {
	sink := &recordingSourceSink{failed: make(chan error, 1)}
	session := startSourceSession(t, 1, sink)
	if routeMappedPhysicalAction(session, nil, func() (control.InputAction, bool, int32, error) {
		panic("private input")
	}) {
		t.Fatal("panicking source mapper suppressed local input")
	}
	select {
	case err := <-sink.failed:
		if !errors.Is(err, ErrSourceMapperPanic) {
			t.Fatalf("Fail() cause = %v", err)
		}
		if err.Error() != ErrSourceMapperPanic.Error() {
			t.Fatalf("panic payload leaked through failure: %q", err)
		}
	case <-time.After(time.Second):
		t.Fatal("source did not report mapper panic")
	}
	if session.Active() {
		t.Fatal("source mapper panic left suppression active")
	}
}

func TestRouteMappedPhysicalActionSuppressesOnlyAcceptedAction(t *testing.T) {
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 1)}
	session := startSourceSession(t, 1, sink)
	action := control.InputAction{Kind: control.InputKeyDown, Code: 1}
	if !routeMappedPhysicalAction(session, nil, func() (control.InputAction, bool, int32, error) {
		return action, true, 0, nil
	}) {
		t.Fatal("accepted source action was not suppressed")
	}
	if got := receiveAction(t, sink.sent); got != action {
		t.Fatalf("sent action = %+v, want %+v", got, action)
	}
}

func TestRouteMappedPhysicalActionKeepsUnmappedEventLocal(t *testing.T) {
	session := newInactiveSourceSession(t)
	if routeMappedPhysicalAction(session, nil, func() (control.InputAction, bool, int32, error) {
		return control.InputAction{}, false, 0, nil
	}) {
		t.Fatal("unmapped event was suppressed")
	}
}
