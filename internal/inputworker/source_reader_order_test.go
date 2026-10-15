package inputworker

import (
	"context"
	"errors"
	"io"
	"testing"
	"testing/synctest"
)

func TestSourceReaderRepliesToHeartbeatWithoutDispatchAndNeverOvertakesCommand(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		run, cancel := context.WithCancel(context.Background())
		defer cancel()
		requests := make(chan sourcePacket, 1)
		completed := make(chan struct{})
		calls, replies := 0, 0
		done := make(chan error, 1)
		go func() {
			done <- pumpSourceCommandsOrdered(run, func() (sourcePacket, error) {
				calls++
				switch calls {
				case 1:
					return sourcePacket{op: sourceActivate, sequence: 1}, nil
				case 2:
					return sourcePacket{op: sourceHeartbeat, sequence: 2}, nil
				case 3:
					return sourcePacket{op: sourceStop, sequence: 3}, nil
				default:
					return sourcePacket{}, io.EOF
				}
			}, requests, cancel, completed, func(p sourcePacket) error {
				if p.sequence != 2 {
					t.Error("heartbeat identity changed")
				}
				replies++
				return nil
			})
		}()
		first := <-requests
		synctest.Wait()
		if first.op != sourceActivate || calls != 1 || replies != 0 {
			t.Fatal("reader ran ahead of pending native command", calls, replies)
		}
		completed <- struct{}{}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if p := <-requests; p.op != sourceStop || calls != 3 || replies != 1 {
			t.Fatal("heartbeat queued native work or stop waited for impossible ACK")
		}
	})
}

func TestSourceReaderFastHeartbeatRetainsWriteFailureAndCancellation(t *testing.T) {
	failure := errors.New("synthetic response failure")
	run, cancel := context.WithCancel(context.Background())
	err := pumpSourceCommandsOrdered(run, func() (sourcePacket, error) { return sourcePacket{op: sourceHeartbeat}, nil }, nil, cancel, make(chan struct{}), func(sourcePacket) error { return failure })
	if err != failure || run.Err() != context.Canceled {
		t.Fatal("heartbeat response failure lost", err)
	}
	cancel()
	synctest.Test(t, func(t *testing.T) {
		run, cancel := context.WithCancel(context.Background())
		requests := make(chan sourcePacket, 1)
		done := make(chan error, 1)
		go func() {
			done <- pumpSourceCommandsOrdered(run, func() (sourcePacket, error) { return sourcePacket{op: sourceActivate}, nil }, requests, cancel, make(chan struct{}), nil)
		}()
		<-requests
		cancel()
		if err := <-done; err != context.Canceled {
			t.Fatal("unconfirmed command could not retire", err)
		}
	})
}
