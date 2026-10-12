package inputworker

import (
	"context"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/agentlifecycle"
	"github.com/0disoft/krasis/internal/application"
)

type ackWaitConn struct {
	net.Conn
	once    sync.Once
	waiting chan struct{}
}

func (c *ackWaitConn) Read(b []byte) (int, error) {
	c.once.Do(func() { close(c.waiting) })
	return c.Conn.Read(b)
}

// A barrier in the independent lifetime proves that the cancellation callback
// has inspected the fully submitted request before the test releases its ACK.
type drainLifetime struct {
	context.Context
	once    sync.Once
	checked chan struct{}
}

func (c *drainLifetime) Err() error {
	c.once.Do(func() { close(c.checked) })
	return c.Context.Err()
}

func awaitClient(t *testing.T, done <-chan error) error {
	t.Helper()
	select {
	case err := <-done:
		return err
	case <-time.After(time.Second):
		t.Fatal("request remained blocked")
		return nil
	}
}

func TestDesktopTransitionDrainsOnlyMatchingSubmittedACK(t *testing.T) {
	for _, event := range []agentlifecycle.Event{agentlifecycle.EventDesktopSwitch, agentlifecycle.EventSessionLock} {
		for _, badACK := range []bool{false, true} {
			t.Run(string(event)+map[bool]string{false: "/matched", true: "/replayed"}[badACK], func(t *testing.T) {
				server, pipe := net.Pipe()
				defer server.Close()
				_ = server.SetDeadline(time.Now().Add(time.Second))
				stream := &ackWaitConn{Conn: pipe, waiting: make(chan struct{})}
				client, err := NewClient(context.Background(), clientChannel(stream, authorityService), testLease())
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				intent := &drainLifetime{Context: context.Background(), checked: make(chan struct{})}
				attempt, cancel := context.WithCancelCause(context.Background())
				defer cancel(context.Canceled)
				ctx := WithDesktopTransitionDrain(attempt, intent)
				done := make(chan error, 1)
				go func() { done <- client.Move(ctx, 1, 0) }()
				command, err := readRequest(server)
				if err != nil {
					t.Fatal(err)
				}
				<-stream.waiting
				cancel(&agentlifecycle.TransitionError{Event: event})
				<-intent.checked
				ack := response{sequence: command.sequence, op: command.op}
				if badACK {
					ack.sequence++
				}
				if err := writeResponse(server, ack); err != nil {
					t.Fatal("transition severed submitted ACK", err)
				}
				err = awaitClient(t, done)
				if !badACK && err != nil {
					t.Fatal("matching ACK discarded", err)
				}
				if badACK {
					if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown {
						t.Fatal("invalid ACK became a successful transition", err)
					}
				}
				if client.sequence != command.sequence+1 {
					t.Fatal("request retransmitted")
				}
			})
		}
	}
}

func TestDesktopDrainPreservesHardStopAndOriginalDeadline(t *testing.T) {
	for _, mode := range []string{"hard_stop", "deadline", "lease_expiry", "manual_cancel", "shutdown", "unmarked", "heartbeat"} {
		t.Run(mode, func(t *testing.T) {
			server, pipe := net.Pipe()
			defer server.Close()
			_ = server.SetDeadline(time.Now().Add(time.Second))
			stream := &ackWaitConn{Conn: pipe, waiting: make(chan struct{})}
			lease := testLease()
			if mode == "lease_expiry" {
				lease.ExpiresAt = time.Now().Add(100 * time.Millisecond)
			}
			client, err := NewClient(context.Background(), clientChannel(stream, authorityService), lease)
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			hardStop, stop := context.WithCancel(context.Background())
			defer stop()
			intent := &drainLifetime{Context: hardStop, checked: make(chan struct{})}
			attempt, cancel := context.WithCancelCause(context.Background())
			defer cancel(context.Canceled)
			ctx := WithDesktopTransitionDrain(attempt, intent)
			if mode == "unmarked" {
				ctx = attempt
			}
			if mode == "deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer deadlineCancel()
			}
			done := make(chan error, 1)
			go func() {
				if mode == "heartbeat" {
					done <- client.Heartbeat(ctx)
				} else {
					done <- client.Move(ctx, 1, 0)
				}
			}()
			if _, err := readRequest(server); err != nil {
				t.Fatal(err)
			}
			<-stream.waiting
			cause := error(&agentlifecycle.TransitionError{Event: agentlifecycle.EventDesktopSwitch})
			if mode == "manual_cancel" {
				cause = context.Canceled
			}
			if mode == "shutdown" {
				cause = &agentlifecycle.TransitionError{Event: agentlifecycle.EventShutdown}
			}
			cancel(cause)
			if mode == "hard_stop" {
				<-intent.checked
				stop()
			}
			err = awaitClient(t, done)
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != application.InjectionOutcomeUnknown {
				t.Fatal("lost ACK certainty changed", err)
			}
		})
	}
}

func TestDesktopDrainNeverWaitsForIncompleteWrite(t *testing.T) {
	server, pipe := net.Pipe()
	defer server.Close()
	client, err := NewClient(context.Background(), clientChannel(pipe, authorityService), testLease())
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	attempt, cancel := context.WithCancelCause(context.Background())
	defer cancel(context.Canceled)
	done := make(chan error, 1)
	go func() { done <- client.Move(WithDesktopTransitionDrain(attempt, context.Background()), 1, 0) }()
	// Read one byte, leaving net.Pipe's write blocked with an incomplete frame.
	var b [1]byte
	_ = server.SetReadDeadline(time.Now().Add(time.Second))
	if _, err := server.Read(b[:]); err != nil {
		t.Fatal(err)
	}
	cancel(&agentlifecycle.TransitionError{Event: agentlifecycle.EventDesktopSwitch})
	if err := awaitClient(t, done); !errors.Is(err, ErrProtocol) {
		t.Fatal(err)
	}
}
