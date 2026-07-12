package wininput

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

func TestSourceSessionSendsAcceptedActionsInOrder(t *testing.T) {
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 2)}
	session := startSourceSession(t, 2, sink)
	if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) ||
		!session.Offer(control.InputAction{Kind: control.InputWheel, DeltaY: 120}) {
		t.Fatal("valid actions were not accepted")
	}
	first := receiveAction(t, sink.sent)
	second := receiveAction(t, sink.sent)
	if first.Kind != control.InputPointerMove || second.Kind != control.InputWheel {
		t.Fatalf("send order = %s, %s", first.Kind, second.Kind)
	}
}

func TestSourceSessionDoesNotSuppressWhileInactive(t *testing.T) {
	sink := &recordingSourceSink{sent: make(chan control.InputAction, 1)}
	session, err := NewSourceSession(1, sink)
	if err != nil {
		t.Fatalf("NewSourceSession() error = %v", err)
	}
	if session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("inactive source session accepted input")
	}
}

func TestSourceSessionOverflowDisablesSuppressionAndFaults(t *testing.T) {
	blockSend := make(chan struct{})
	sink := &recordingSourceSink{blockSend: blockSend, sent: make(chan control.InputAction, 1), failed: make(chan error, 1)}
	session := startSourceSession(t, 1, sink)
	if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("first action was not accepted")
	}
	receiveAction(t, sink.sent)
	if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 2}) {
		t.Fatal("queued action was not accepted")
	}
	if session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 3}) {
		t.Fatal("overflowing action was incorrectly suppressed")
	}
	if session.Active() {
		t.Fatal("overflow did not disable suppression immediately")
	}
	close(blockSend)
	select {
	case err := <-sink.failed:
		if !errors.Is(err, ErrSourceQueueOverflow) {
			t.Fatalf("fault = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("source fault was not delivered")
	}
	select {
	case action := <-sink.sent:
		t.Fatalf("stale queued action was sent: %+v", action)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestSourceSessionInvalidActionFailsOpen(t *testing.T) {
	sink := &recordingSourceSink{failed: make(chan error, 1)}
	session := startSourceSession(t, 1, sink)
	if session.Offer(control.InputAction{Kind: control.InputPointerMove}) {
		t.Fatal("invalid action was suppressed")
	}
	if session.Active() {
		t.Fatal("invalid action left suppression active")
	}
	select {
	case err := <-sink.failed:
		if !errors.Is(err, control.ErrInvalidInputAction) {
			t.Fatalf("fault = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("invalid action fault was not delivered")
	}
}

func TestSourceSessionSendFailureDisablesSuppression(t *testing.T) {
	sendErr := errors.New("send failed")
	sink := &recordingSourceSink{sendErr: sendErr, failed: make(chan error, 1)}
	session := startSourceSession(t, 1, sink)
	if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("action was not accepted")
	}
	select {
	case err := <-sink.failed:
		if !errors.Is(err, sendErr) {
			t.Fatalf("fault = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("send failure was not delivered")
	}
	if session.Active() {
		t.Fatal("send failure left suppression active")
	}
}

func TestSourceSessionInterruptsBlockedSendBeforeFaultCleanup(t *testing.T) {
	sink := &interruptibleSourceSink{
		started:     make(chan struct{}),
		interrupted: make(chan struct{}),
		failed:      make(chan error, 1),
	}
	session := startSourceSession(t, 1, sink)
	action := control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}
	if !session.Offer(action) {
		t.Fatal("first action was not accepted")
	}
	select {
	case <-sink.started:
	case <-time.After(time.Second):
		t.Fatal("SendAction did not start")
	}
	if !session.Offer(action) {
		t.Fatal("queued action was not accepted")
	}
	if session.Offer(action) {
		t.Fatal("overflowing action was incorrectly accepted")
	}
	select {
	case cause := <-sink.failed:
		if !errors.Is(cause, ErrSourceQueueOverflow) {
			t.Fatalf("cleanup cause = %v", cause)
		}
	case <-time.After(time.Second):
		t.Fatal("blocked send prevented fault cleanup")
	}
}

func startSourceSession(t *testing.T, capacity int, sink SourceSink) *SourceSession {
	t.Helper()
	session, err := NewSourceSession(capacity, sink)
	if err != nil {
		t.Fatalf("NewSourceSession() error = %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- session.Run(ctx) }()
	t.Cleanup(func() {
		cancel()
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Fatal("source session did not stop")
		}
	})
	deadline := time.Now().Add(time.Second)
	for !session.started.Load() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if err := session.Activate(); err != nil {
		t.Fatalf("Activate() error = %v", err)
	}
	return session
}

func receiveAction(t *testing.T, actions <-chan control.InputAction) control.InputAction {
	t.Helper()
	select {
	case action := <-actions:
		return action
	case <-time.After(time.Second):
		t.Fatal("action was not sent")
		return control.InputAction{}
	}
}

type recordingSourceSink struct {
	sent      chan control.InputAction
	failed    chan error
	blockSend <-chan struct{}
	sendErr   error
}

func (sink *recordingSourceSink) SendAction(_ context.Context, action control.InputAction) error {
	if sink.sent != nil {
		sink.sent <- action
	}
	if sink.blockSend != nil {
		<-sink.blockSend
	}
	return sink.sendErr
}

func (*recordingSourceSink) Interrupt(error) error { return nil }

func (sink *recordingSourceSink) Fail(_ context.Context, cause error) error {
	if sink.failed != nil {
		sink.failed <- cause
	}
	return nil
}

type interruptibleSourceSink struct {
	startedOnce   sync.Once
	interruptOnce sync.Once
	started       chan struct{}
	interrupted   chan struct{}
	failed        chan error
}

func (sink *interruptibleSourceSink) SendAction(context.Context, control.InputAction) error {
	sink.startedOnce.Do(func() { close(sink.started) })
	<-sink.interrupted
	return errors.New("send interrupted")
}

func (sink *interruptibleSourceSink) Interrupt(error) error {
	sink.interruptOnce.Do(func() { close(sink.interrupted) })
	return nil
}

func (sink *interruptibleSourceSink) Fail(_ context.Context, cause error) error {
	sink.failed <- cause
	return nil
}
