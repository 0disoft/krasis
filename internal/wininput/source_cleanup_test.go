package wininput

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestSourceSenderCancellationCannotEraseJoinedCleanupFailure(t *testing.T) {
	for _, expiry := range []bool{false, true} {
		t.Run(fmt.Sprint("expiry=", expiry), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			if expiry {
				var stop context.CancelFunc
				ctx, stop = context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer stop()
			}
			session, err := NewSourceSession(1, &recordingSourceSink{})
			if err != nil {
				t.Fatal(err)
			}
			sender := make(chan error, 1)
			independent := errors.New("independent cleanup failure")
			sender <- fmt.Errorf("sender: %w", errors.Join(ctx.Err(), errors.Join(inputdesktop.ErrCleanup, independent)))
			result := session.cleanupSession(ctx, nil, sender, ctx.Err(), false)
			if !errors.Is(result, inputdesktop.ErrCleanup) || !errors.Is(result, independent) {
				t.Fatal("sender cleanup failure erased", result)
			}
		})
	}
}

type stalledSourceCleanupSink struct {
	blockInterrupt             bool
	entered, release, finished chan struct{}
}

func (*stalledSourceCleanupSink) SendAction(context.Context, control.InputAction, func() bool) error {
	return nil
}
func (s *stalledSourceCleanupSink) stall() error {
	close(s.entered)
	<-s.release
	close(s.finished)
	return nil
}
func (s *stalledSourceCleanupSink) Interrupt(error) error {
	if s.blockInterrupt {
		return s.stall()
	}
	return nil
}
func (s *stalledSourceCleanupSink) Fail(context.Context, error) error { return s.stall() }

func TestSourceCleanupTimeoutMarksUnjoinedInterruptAndFailure(t *testing.T) {
	for _, interrupt := range []bool{false, true} {
		t.Run(fmt.Sprint("interrupt=", interrupt), func(t *testing.T) {
			sink := &stalledSourceCleanupSink{blockInterrupt: interrupt, entered: make(chan struct{}), release: make(chan struct{}), finished: make(chan struct{})}
			defer func() {
				close(sink.release)
				select {
				case <-sink.finished:
				case <-time.After(time.Second):
					t.Error("fixture did not join")
				}
			}()
			session, err := NewSourceSession(1, sink)
			if err != nil {
				t.Fatal(err)
			}
			session.cleanup = 25 * time.Millisecond
			ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
			defer cancel()
			result := session.cleanupSession(ctx, nil, nil, ctx.Err(), true)
			if !errors.Is(result, ErrSourceCleanupIncomplete) || !errors.Is(result, context.DeadlineExceeded) {
				t.Fatal("independent cleanup timeout lost", result)
			}
		})
	}
}

func TestRevokedGenerationCannotEraseUnknownOrJoinedSendFailures(t *testing.T) {
	unknown := errors.New("transmission completion unknown")
	for _, failure := range []error{unknown, inputdesktop.ErrCleanup, errors.Join(context.Canceled, inputdesktop.ErrCleanup), errors.Join(control.ErrSourceActionDiscarded, unknown)} {
		sink := &revokedGenerationSourceSink{firstStarted: make(chan struct{}), releaseFirst: make(chan struct{}), sent: make(chan control.InputAction, 1), failed: make(chan error, 1), firstError: failure}
		session := startSourceSession(t, 2, sink)
		if !session.Offer(control.InputAction{Kind: control.InputPointerMove, DeltaX: 1}) {
			t.Fatal("not offered")
		}
		select {
		case <-sink.firstStarted:
		case <-time.After(time.Second):
			t.Fatal("sender did not start")
		}
		session.Deactivate()
		close(sink.releaseFirst)
		select {
		case observed := <-sink.failed:
			if !errors.Is(observed, failure) {
				t.Fatal("revoked generation erased actual send failure", observed)
			}
		case <-time.After(time.Second):
			t.Fatal("revoked generation send failure was discarded")
		}
	}
}
