package inputworker

import (
	"context"
	"errors"
	"io"
	"net"
	"os"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

type admissionObservedConn struct {
	net.Conn
	writes atomic.Int32
}

type rejectedAdmissionConn struct {
	net.Conn
	n       int
	failure error
}

func (stream *rejectedAdmissionConn) Write([]byte) (int, error) {
	return stream.n, stream.failure
}

func TestClientUsesOnlyNativeNonSubmissionProofForWriteCertainty(t *testing.T) {
	for _, example := range []struct {
		name    string
		n       int
		failure error
		want    application.InjectionCertainty
	}{
		{"native_admission", 0, pipeAdmissionFailure(false, os.ErrDeadlineExceeded), application.InjectionNotApplied},
		{"ordinary_timeout", 0, os.ErrDeadlineExceeded, application.InjectionOutcomeUnknown},
		{"transport_loss", 0, io.ErrClosedPipe, application.InjectionOutcomeUnknown},
		{"partial_write", 1, pipeAdmissionFailure(false, os.ErrDeadlineExceeded), application.InjectionOutcomeUnknown},
	} {
		t.Run(example.name, func(t *testing.T) {
			server, pipe := net.Pipe()
			defer server.Close()
			stream := &rejectedAdmissionConn{Conn: pipe, n: example.n, failure: example.failure}
			client, err := NewClient(context.Background(), clientChannel(stream, authorityService), testLease())
			if err != nil {
				t.Fatal(err)
			}
			defer client.Close()
			err = client.Move(context.Background(), 1, 0)
			if certainty, ok := application.InjectionCertaintyOf(err); !ok || certainty != example.want || !client.failed {
				t.Fatal("write certainty ignored submission evidence or reused failed channel", err, certainty)
			}
			if example.want == application.InjectionNotApplied && !errors.Is(err, os.ErrDeadlineExceeded) {
				t.Fatal("native admission lost the original deadline failure", err)
			}
		})
	}
}

func (stream *admissionObservedConn) Write(data []byte) (int, error) {
	stream.writes.Add(1)
	return stream.Conn.Write(data)
}

func TestClientRechecksCancellationAndExpiryAfterAuthorization(t *testing.T) {
	for _, mode := range []string{"cancellation", "deadline", "lease_expiry"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				server, pipe := net.Pipe()
				defer server.Close()
				stream := &admissionObservedConn{Conn: pipe}
				channel := clientChannel(stream, authorityService)
				lease := testLease()
				if mode == "lease_expiry" {
					lease.ExpiresAt = time.Now().Add(time.Second)
				}
				client, err := NewClient(context.Background(), channel, lease)
				if err != nil {
					t.Fatal(err)
				}
				defer client.Close()
				ctx, cancel := context.WithCancel(context.Background())
				defer cancel()
				channel.authority.checkInputObserved = func(budget context.Context) error {
					switch mode {
					case "cancellation":
						cancel()
					case "deadline":
						<-budget.Done()
					case "lease_expiry":
						time.Sleep(time.Until(lease.ExpiresAt))
					}
					return nil
				}
				err = client.Move(ctx, 1, 0)
				certainty, ok := application.InjectionCertaintyOf(err)
				if !ok || certainty != application.InjectionNotApplied || stream.writes.Load() != 0 || client.sequence != 1 {
					t.Fatal("expired authorization attempted input or lost not-sent certainty", err, certainty, stream.writes.Load(), client.sequence)
				}
				cause := error(context.Canceled)
				if mode == "deadline" {
					cause = context.DeadlineExceeded
				}
				if mode == "lease_expiry" {
					if !errors.Is(err, ErrExpired) || !client.failed {
						t.Fatal("expired lease was left usable", err)
					}
					return
				}
				if !errors.Is(err, cause) {
					t.Fatal("unsent cancellation lost its cause", err)
				}
				if err := client.Move(context.Background(), 2, 0); err == nil || stream.writes.Load() != 0 || client.sequence != 1 {
					t.Fatal("expired proof accepted fresh input or replayed cancellation", err)
				}
			})
		})
	}
}
