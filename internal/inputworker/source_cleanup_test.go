package inputworker

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
)

type blockedCleanupSourceSink struct {
	entered, release, finished chan struct{}
	returned                   atomic.Bool
}

func TestSourceIncompleteCleanupCannotHideBehindExpiryOrTransport(t *testing.T) {
	for _, sender := range []error{
		errors.Join(context.DeadlineExceeded, wininput.ErrSourceCleanupIncomplete),
		sourceTransportFailure(errors.Join(context.DeadlineExceeded, wininput.ErrSourceCleanupIncomplete)),
		fmt.Errorf("wrapped: %w", errors.Join(context.Canceled, sourceTransportFailure(wininput.ErrSourceCleanupIncomplete))),
	} {
		result := sourceCaptureFinal(context.DeadlineExceeded, inputdesktop.ErrInactive, sender, true)
		if !errors.Is(result, ErrRecovery) || !errors.Is(result, wininput.ErrSourceCleanupIncomplete) || sourceRetirementStatus(result) != 1 {
			t.Fatal("incomplete cleanup became retirement proof", result)
		}
	}
	if result := sourceCaptureFinal(context.DeadlineExceeded, inputdesktop.ErrInactive, context.DeadlineExceeded, true); result != nil {
		t.Fatal("clean lease expiry became incomplete cleanup", result)
	}
}

func (s *blockedCleanupSourceSink) SendAction(context.Context, control.InputAction, func() bool) error {
	close(s.entered)
	<-s.release
	s.returned.Store(true)
	close(s.finished)
	return nil
}
func (*blockedCleanupSourceSink) Interrupt(error) error             { return nil }
func (*blockedCleanupSourceSink) Fail(context.Context, error) error { return nil }

func TestLeaseExpiryCannotCertifyUnjoinedSourceSender(t *testing.T) {
	sink := &blockedCleanupSourceSink{entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
	defer func() {
		close(sink.release)
		select {
		case <-sink.finished:
		case <-time.After(time.Second):
			t.Error("fixture sender did not exit")
		}
	}()
	session, err := wininput.NewSourceSession(4, sink)
	if err != nil {
		t.Fatal(err)
	}
	run, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- session.RunWithReady(run, func() { close(ready) }) }()
	<-ready
	if err := session.Activate(); err != nil {
		t.Fatal(err)
	}
	if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
		t.Fatal("not offered")
	}
	select {
	case <-sink.entered:
	case <-time.After(time.Second):
		t.Fatal("sender did not enter")
	}
	var sender error
	select {
	case sender = <-done:
	case <-time.After(4 * time.Second):
		t.Fatal("cleanup not bounded")
	}
	if sink.returned.Load() {
		t.Fatal("sender must remain blocked for reproduction")
	}
	result := sourceCaptureFinal(run.Err(), inputdesktop.ErrInactive, sender, true)
	var wire bytes.Buffer
	if err := writeReport(&wire, Report{}, result); err != nil {
		t.Fatal(err)
	}
	status := wire.Bytes()[5]
	report, received := readReport(&wire)
	_, received = receipt(report, received)
	t.Logf("senderReturned=%v retirementStatus=%d reportStatus=%d serviceFence=%v", sink.returned.Load(), sourceRetirementStatus(result), status, serviceRecoveryFailure(received, nil))
	if !errors.Is(result, ErrRecovery) || sourceRetirementStatus(result) != 1 || status != 2 || serviceRecoveryFailure(received, nil) == nil {
		t.Fatal("running sender certified as clean retirement", result)
	}
}
