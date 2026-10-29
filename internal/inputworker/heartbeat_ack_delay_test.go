package inputworker

import (
	"context"
	"net"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/control"
)

// The worker has already delivered its ACK and renewed its idle deadline.
// Delay only the client's return from that read, as when its thread is descheduled.
type delayedACKConn struct {
	net.Conn
	delay time.Duration
}

func (c *delayedACKConn) Read(p []byte) (int, error) {
	n, err := c.Conn.Read(p)
	if n > 0 && c.delay != 0 {
		delay := c.delay
		c.delay = 0
		time.Sleep(delay)
	}
	return n, err
}

func TestInputHeartbeatDoesNotTreatDelayedACKAsFreshTraffic(t *testing.T) {
	for _, delay := range []time.Duration{0, 300 * time.Millisecond, 1900 * time.Millisecond} {
		t.Run(delay.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				input, lease := &recordingInput{}, testLease()
				conn, done := startWorker(t, context.Background(), input, lease)
				client, err := NewClient(context.Background(), clientChannel(&delayedACKConn{Conn: conn, delay: delay}, authorityService), lease)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				key := control.PressedToken{Kind: control.PressedKeyboardKey, Code: 65}
				if err := client.Press(context.Background(), key); err != nil {
					t.Fatal("initial input was not acknowledged", err)
				}
				if err := client.Heartbeat(context.Background()); err != nil {
					t.Fatal("maintenance failed after a matching ACK", err)
				}
				wantSequence := uint64(3)
				if delay == 0 {
					wantSequence = 2 // Fresh ACK still avoids an unnecessary roundtrip.
				}
				if client.sequence != wantSequence {
					t.Errorf("ACK processing delay %s suppressed required maintenance: sequence=%d, want %d", delay, client.sequence, wantSequence)
				}
				time.Sleep(250 * time.Millisecond)
				if err := client.Heartbeat(context.Background()); err != nil {
					t.Errorf("worker expired despite successful input and scheduled maintenance: %v", err)
				}
				_ = client.Close()
				result := <-done
				if result.err != nil || result.report.Completed != 1 || input.pressed != 1 || input.released != 1 || result.report.Recovery.Remaining != 0 {
					t.Fatalf("idle maintenance lost worker/release ownership: report=%+v error=%v pressed=%d released=%d", result.report, result.err, input.pressed, input.released)
				}
			})
		})
	}
}
