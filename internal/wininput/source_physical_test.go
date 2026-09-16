package wininput

import (
	"github.com/0disoft/krasis/internal/control"
	"testing"
)

func TestLocalPressCannotCrossGrantBoundary(t *testing.T) {
	for _, kinds := range [][2]control.InputActionKind{{control.InputKeyDown, control.InputKeyUp}, {control.InputButtonDown, control.InputButtonUp}} {
		t.Run(string(kinds[0]), func(t *testing.T) {
			session, err := NewSourceSession(4, &recordingSourceSink{})
			if err != nil {
				t.Fatal(err)
			}
			session.started.Store(true)
			down := control.InputAction{Kind: kinds[0], Code: 1}
			up := control.InputAction{Kind: kinds[1], Code: 1}
			if routePhysicalAction(session, nil, down, 0) {
				t.Fatal("local down suppressed")
			}
			if routePhysicalAction(session, nil, down, 0) {
				t.Fatal("local repeat suppressed")
			}
			if err := session.Activate(); err == nil {
				t.Fatal("grant accepted held local press")
			}
			if routePhysicalAction(session, nil, up, 0) {
				t.Fatal("local up became remote")
			}
			if len(session.queue) != 0 {
				t.Fatal("local press leaked into remote queue")
			}
			if err := session.Activate(); err != nil {
				t.Fatal(err)
			}
			if !routePhysicalAction(session, nil, down, 0) || !routePhysicalAction(session, nil, up, 0) {
				t.Fatal("remote pair not accepted")
			}
			if len(session.queue) != 2 {
				t.Fatal("remote pair not queued exactly once")
			}
		})
	}
}

func TestActivationChecksPreviouslyHeldNativeInput(t *testing.T) {
	session, _ := NewSourceSession(1, &recordingSourceSink{})
	session.started.Store(true)
	session.physicalCheck = func() bool { return true }
	if err := session.Activate(); err == nil {
		t.Fatal("pre-hook held input accepted")
	}
}
