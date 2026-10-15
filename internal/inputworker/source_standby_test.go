package inputworker

import (
	"context"
	"errors"
	"net"
	"os"
	"testing"
	"testing/synctest"
	"time"
)

func TestSourceStandbyDoesNotTreatIdleSchedulerPauseAsActiveCaptureFailure(t *testing.T) {
	for _, active := range []bool{false, true} {
		synctest.Test(t, func(t *testing.T) {
			server, client := net.Pipe()
			defer server.Close()
			defer client.Close()
			deadline := time.Now().Add(sourceReadTimeout(true, active))
			if err := server.SetReadDeadline(deadline); err != nil {
				t.Fatal(err)
			}
			data, err := encodeSourcePacket(sourcePacket{kind: sourceCommand, op: sourceHeartbeat, binding: Binding{1}, sequence: 1})
			if err != nil {
				t.Fatal(err)
			}
			written := make(chan struct{})
			go func() {
				defer close(written)
				time.Sleep(IdleTimeout + 100*time.Millisecond)
				_, _ = client.Write(data[:])
			}()
			defer func() { _ = server.Close(); _ = client.Close(); <-written }()
			frame := &sourceFrameReader{Reader: server, setDeadline: server.SetReadDeadline, deadline: deadline}
			_, err = readSourceCommand(frame, context.Background(), Binding{1}, 0, func() error { return nil })
			if active {
				if !errors.Is(err, os.ErrDeadlineExceeded) {
					t.Fatal("active capture outlived its original bound", err)
				}
			} else if err != nil {
				t.Fatal("idle owner was retired for a delayed keepalive", err)
			}
		})
	}
	if sourceReadTimeout(false, false) != IdleTimeout {
		t.Fatal("startup gained standby grace")
	}
}

func TestSourceStandbyPartialFrameStillExpiresWithinActiveReadBudget(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		server, client := net.Pipe()
		defer server.Close()
		defer client.Close()
		deadline := time.Now().Add(sourceReadTimeout(true, false))
		_ = server.SetReadDeadline(deadline)
		go func() { _, _ = client.Write([]byte{'K'}) }()
		start := time.Now()
		frame := &sourceFrameReader{Reader: server, setDeadline: server.SetReadDeadline, deadline: deadline}
		_, err := readSourceCommand(frame, context.Background(), Binding{1}, 0, func() error { t.Fatal("partial frame authorized"); return nil })
		if !errors.Is(err, os.ErrDeadlineExceeded) || time.Since(start) != IdleTimeout {
			t.Fatal("partial standby frame escaped its bound", err, time.Since(start))
		}
	})
}

func TestSourcePresenceLossStopsStandbyWithoutWaitingForIdleDeadline(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		calls := 0
		start := time.Now()
		err := watchSourcePresence(context.Background(), func() error {
			calls++
			if calls == 2 {
				return ErrAuthority
			}
			return nil
		})
		if err != ErrAuthority || calls != 2 || time.Since(start) != 500*time.Millisecond {
			t.Fatal("presence loss waited for idle expiry", err, calls, time.Since(start))
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := watchSourcePresence(ctx, func() error { t.Fatal("queried after cancellation"); return nil }); err != nil {
			t.Fatal(err)
		}
	})
}
