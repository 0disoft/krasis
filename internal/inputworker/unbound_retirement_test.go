package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"os"
	"syscall"
	"testing"
	"testing/synctest"
	"time"
)

func TestUnboundRetirementHonorsCompletedStopAfterStartupExpires(t *testing.T) {
	// The exact authenticated zero bootstrap was fully read, and its startup
	// callback disarmed, before the worker was descheduled past startup expiry.
	// Retirement grants no input; rejecting it here loses the final receipt.
	startup, cancel := context.WithCancel(context.Background())
	cancel()
	conn := &unboundRetirementConn{}
	retired := false
	err := retireUnboundNative(startup, conn, func() bool { return true }, func() error {
		retired = true
		return nil
	})
	if err != nil || !retired || !bytes.Equal(conn.written.Bytes(), unboundRetiredFrame[:]) {
		t.Fatal("completed authenticated stop lost to late startup cancellation", err, retired)
	}
}

func TestUnboundRetirementReceiptSurvivesStartupExpiryDuringCleanup(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service, worker := net.Pipe()
		defer service.Close()
		defer worker.Close()
		startup, cancel := context.WithTimeout(context.Background(), startupTimeout)
		defer cancel()
		if err := worker.SetDeadline(time.Now().Add(startupTimeout)); err != nil {
			t.Fatal(err)
		}
		stopStartupIO := context.AfterFunc(startup, func() { _ = worker.Close() })
		defer stopStartupIO()
		finished := make(chan error, 1)
		retired := false
		go func() {
			if err := writeFrame(worker, workerAuthenticatedFrame[:]); err != nil {
				finished <- err
				return
			}
			var stop [bootstrapSize]byte
			if _, err := io.ReadFull(worker, stop[:]); err != nil {
				finished <- err
				return
			}
			if !zeroBytes(stop[:]) {
				finished <- ErrProtocol
				return
			}
			finished <- retireUnboundNative(startup, worker, stopStartupIO, func() error {
				// Below the native close's four-second bound, but crossing the
				// original startup deadline after the two-second warm window.
				time.Sleep(3500 * time.Millisecond)
				retired = true
				return nil
			})
		}()
		time.Sleep(2 * time.Second)
		err := retireInputPreparation(service, workerAuthenticatedFrame, unboundRetiredFrame)
		childErr := <-finished
		if err != nil || childErr != nil || !retired {
			t.Fatal("startup cancellation cut off confirmed unbound retirement", err, childErr, retired)
		}
		if !errors.Is(startup.Err(), context.DeadlineExceeded) {
			t.Fatal("fixture did not cross startup expiry")
		}
	})
}

func TestUnboundRetirementReceiptHasFullBudgetAfterSlowAuthentication(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service, worker := net.Pipe()
		defer service.Close()
		defer worker.Close()
		startup, cancel := context.WithTimeout(context.Background(), startupTimeout)
		defer cancel()
		stopStartupIO := context.AfterFunc(startup, func() { _ = worker.Close() })
		defer stopStartupIO()
		finished := make(chan error, 1)
		go func() {
			// Native preparation can finish after the service starts retiring.
			time.Sleep(2 * time.Second)
			if err := writeFrame(worker, workerAuthenticatedFrame[:]); err != nil {
				finished <- err
				return
			}
			var stop [bootstrapSize]byte
			if _, err := io.ReadFull(worker, stop[:]); err != nil || !zeroBytes(stop[:]) {
				finished <- errors.Join(ErrProtocol, err)
				return
			}
			finished <- retireUnboundNative(startup, worker, stopStartupIO, func() error {
				// Valid cleanup within the native four-second close bound.
				time.Sleep(3500 * time.Millisecond)
				return nil
			})
		}()
		started := time.Now()
		parentErr := retireInputPreparation(service, workerAuthenticatedFrame, unboundRetiredFrame)
		childErr := <-finished
		if parentErr != nil || childErr != nil {
			t.Fatal("authentication consumed the native retirement receipt budget", parentErr, childErr)
		}
		if time.Since(started) != 5500*time.Millisecond {
			t.Fatal("fixture did not cross the original shared deadline")
		}
	})
}

type unboundRetirementConn struct {
	net.Conn
	written               bytes.Buffer
	deadline              time.Time
	deadlineErr, writeErr error
}

func TestUnboundRetirementSlowAuthenticationStillExpires(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service, worker := net.Pipe()
		defer service.Close()
		defer worker.Close()
		finished := make(chan error, 1)
		go func() {
			time.Sleep(startupTimeout + time.Second)
			finished <- writeFrame(worker, workerAuthenticatedFrame[:])
		}()
		started := time.Now()
		err := retireInputPreparation(service, workerAuthenticatedFrame, unboundRetiredFrame)
		if !errors.Is(err, ErrRecovery) || !errors.Is(err, os.ErrDeadlineExceeded) || time.Since(started) != startupTimeout {
			t.Fatal("authentication timeout was extended", err)
		}
		_ = service.Close()
		if <-finished == nil {
			t.Fatal("late authentication was accepted")
		}
	})
}

func (c *unboundRetirementConn) SetDeadline(deadline time.Time) error {
	c.deadline = deadline
	return c.deadlineErr
}

func (c *unboundRetirementConn) Write(data []byte) (int, error) {
	if c.writeErr != nil {
		return 0, c.writeErr
	}
	return c.written.Write(data)
}

func TestUnboundRetirementCannotAcknowledgeExpiredFailedOrUnconfirmedCleanup(t *testing.T) {
	for _, tc := range []struct {
		name                                   string
		cancelled, callbackRunning             bool
		deadlineErr, retireErr, writeErr, want error
		retireCalls                            int
	}{
		{name: "startup already cancelled and callback running", cancelled: true, callbackRunning: true, want: context.Canceled},
		{name: "startup callback already running", callbackRunning: true},
		{name: "deadline rejected", deadlineErr: syscall.Errno(6), want: syscall.Errno(6)},
		{name: "unconfirmed native exit", retireErr: syscall.Errno(5), want: syscall.Errno(5), retireCalls: 1},
		{name: "receipt lost", writeErr: io.ErrClosedPipe, want: io.ErrClosedPipe, retireCalls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			startup, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}
			conn := &unboundRetirementConn{deadlineErr: tc.deadlineErr, writeErr: tc.writeErr}
			retired, stopped := 0, 0
			err := retireUnboundNative(startup, conn, func() bool { stopped++; return !tc.callbackRunning }, func() error { retired++; return tc.retireErr })
			if !errors.Is(err, ErrRecovery) || tc.want != nil && !errors.Is(err, tc.want) || stopped != 1 || retired != tc.retireCalls || conn.written.Len() != 0 {
				t.Fatal("uncertain startup/retirement became an acknowledgement", err, stopped, retired, conn.written.Len())
			}
		})
	}
}

func TestUnboundRetirementUsesBoundedFreshDeadlineAndExactReceipt(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		conn := &unboundRetirementConn{}
		started := time.Now()
		stopped := false
		err := retireUnboundNative(context.Background(), conn, func() bool { stopped = true; return true }, func() error {
			if !stopped || !conn.deadline.Equal(started.Add(startupTimeout)) {
				t.Fatal("cleanup began without retiring startup IO or a bounded deadline")
			}
			return nil
		})
		if err != nil || !bytes.Equal(conn.written.Bytes(), unboundRetiredFrame[:]) {
			t.Fatal("retirement receipt changed", err)
		}
	})
}

func TestUnboundRetirementCannotDeliverReceiptAfterRetirementDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service, worker := net.Pipe()
		defer service.Close()
		defer worker.Close()
		finished := make(chan error, 1)
		go func() {
			if err := writeFrame(worker, workerAuthenticatedFrame[:]); err != nil {
				finished <- err
				return
			}
			var stop [bootstrapSize]byte
			if _, err := io.ReadFull(worker, stop[:]); err != nil {
				finished <- err
				return
			}
			finished <- retireUnboundNative(context.Background(), worker, func() bool { return true }, func() error {
				time.Sleep(startupTimeout + time.Second)
				return nil
			})
		}()
		parentErr := retireInputPreparation(service, workerAuthenticatedFrame, unboundRetiredFrame)
		childErr := <-finished
		if !errors.Is(parentErr, ErrRecovery) || !errors.Is(parentErr, os.ErrDeadlineExceeded) || !errors.Is(childErr, ErrRecovery) || !errors.Is(childErr, os.ErrDeadlineExceeded) {
			t.Fatal("late cleanup bypassed bounded receipt or recovery fence", parentErr, childErr)
		}
	})
}
