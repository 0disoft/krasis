//go:build windows

package inputworker

import (
	"context"
	"errors"
	"io"
	"net"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"
)

func TestPreparationReceiptAuthoritySharesResponseBudget(t *testing.T) {
	for _, budget := range []time.Duration{50 * time.Millisecond, time.Second} {
		t.Run(budget.String(), func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				defer isolateAuthorityCapacity(t)()
				peer, pipe := net.Pipe()
				defer peer.Close()
				stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
				channel := clientChannel(stream, authorityAgent)
				defer channel.Close()
				var closed atomic.Int32
				channel.authority.closeHandles = func() error { closed.Add(1); return nil }
				channel.authority.checkInput = func() error {
					time.Sleep(2 * budget)
					if closed.Load() != 0 || stream.closes.Load() != 0 {
						t.Error("receipt released handles still owned by a native check")
					}
					return nil
				}
				go func() { var receipt [8]byte; _, _ = io.ReadFull(peer, receipt[:]) }()
				ctx := context.Background()
				if budget < time.Second {
					var cancel context.CancelFunc
					ctx, cancel = context.WithTimeout(ctx, budget)
					defer cancel()
				}
				started := time.Now()
				err := acknowledgePreparation(ctx, channel)
				if !errors.Is(err, ErrAuthority) || time.Since(started) != budget {
					t.Errorf("receipt authorization escaped budget: elapsed=%s err=%v", time.Since(started), err)
				}
				if !channel.authority.fenced.Load() || channel.Close() != ErrAuthority || closed.Load() != 0 {
					t.Error("unfinished receipt proof lost its exact cleanup owner")
				}
				time.Sleep(2 * budget)
				synctest.Wait()
				if channel.Close() != nil || closed.Load() != 1 || stream.closes.Load() != 1 {
					t.Error("late receipt query failed exact retirement")
				}
			})
		})
	}
}

func TestPreparationReceiptWriteUsesRemainingBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer, pipe := net.Pipe()
		defer peer.Close()
		channel := clientChannel(pipe, authorityAgent)
		defer channel.Close()
		channel.authority.checkInput = func() error { time.Sleep(25 * time.Millisecond); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := time.Now()
		if err := acknowledgePreparation(ctx, channel); err == nil || time.Since(started) != 50*time.Millisecond {
			t.Fatal("receipt write restarted the budget", time.Since(started), err)
		}
	})
}

func TestPreparationReceiptPreservesFreshProofAndExactBytes(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		peer, pipe := net.Pipe()
		defer peer.Close()
		stream := &queryOwnedConn{admissionObservedConn: &admissionObservedConn{Conn: pipe}}
		channel := clientChannel(stream, authorityAgent)
		defer channel.Close()
		var checks atomic.Int32
		channel.authority.checkInput = func() error { checks.Add(1); return nil }
		done := make(chan error, 1)
		go func() {
			var packet [8]byte
			_, err := io.ReadFull(peer, packet[:])
			if err == nil && packet != preparationReceipt {
				err = ErrProtocol
			}
			done <- err
		}()
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()
		if err := acknowledgePreparation(ctx, channel); err != nil {
			t.Fatal(err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		cancel()
		time.Sleep(2 * time.Second)
		synctest.Wait()
		if checks.Load() != 1 || stream.closes.Load() != 0 || channel.authority.fenced.Load() {
			t.Fatal("receipt skipped its fresh proof or retained cancellation after success")
		}
	})
}

func TestPreparationReceiptCannotBypassNativeCapacity(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		defer isolateAuthorityCapacity(t)()
		_, finish := occupyAuthorityQueries(t)
		defer finish()
		peer, pipe := net.Pipe()
		defer peer.Close()
		channel := clientChannel(pipe, authorityAgent)
		defer channel.Close()
		var checks atomic.Int32
		channel.authority.checkInput = func() error { checks.Add(1); return nil }
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()
		started := time.Now()
		err := acknowledgePreparation(ctx, channel)
		if !errors.Is(err, ErrAuthority) || !errors.Is(err, context.DeadlineExceeded) || time.Since(started) != 50*time.Millisecond || checks.Load() != 0 {
			t.Fatal("receipt bypassed native capacity or lost caller budget", checks.Load(), err)
		}
	})
}

type receiptDeadlineFailureConn struct {
	net.Conn
	writes int
}

func (c *receiptDeadlineFailureConn) SetWriteDeadline(time.Time) error { return io.ErrClosedPipe }
func (c *receiptDeadlineFailureConn) Write([]byte) (int, error) {
	c.writes++
	return 0, io.ErrClosedPipe
}

func TestPreparationReceiptDeadlineFailureCannotWrite(t *testing.T) {
	peer, pipe := net.Pipe()
	defer peer.Close()
	stream := &receiptDeadlineFailureConn{Conn: pipe}
	channel := clientChannel(stream, authorityAgent)
	defer channel.Close()
	err := acknowledgePreparation(context.Background(), channel)
	if !errors.Is(err, io.ErrClosedPipe) || stream.writes != 0 {
		t.Fatal("receipt wrote without its mandatory deadline", stream.writes, err)
	}
}
