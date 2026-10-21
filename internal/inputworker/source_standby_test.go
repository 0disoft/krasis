package inputworker

import (
	"context"
	"errors"
	"net"
	"os"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/inputdesktop"
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

func TestSourcePresenceWatchRetainsSanitizedFailureInServiceReport(t *testing.T) {
	for _, operation := range []string{"authority-process-exited", "authority-console-session", "authority-service-pipe"} {
		synctest.Test(t, func(t *testing.T) {
			cause := authorityCheckFailed(operation, syscall.Errno(5))
			observed := watchSourcePresence(context.Background(), func() error { return cause })
			if !errors.Is(observed, ErrAuthority) {
				t.Fatal("watcher lost authority denial", observed)
			}
			loop := &sourceReaderFailure{operation: "source-reader-authority", cause: observed}
			err := sourceCaptureFinal(loop, inputdesktop.ErrInactive, context.Canceled, false)
			code, native := reportFailureDetails(err)
			if reportOperations[code] != operation || native != 5 {
				t.Fatalf("watcher erased native failure: operation=%s native=%d", reportOperations[code], native)
			}
			if sourceRetirementStatus(err) != 0 || sourceRetirementStatus(errors.Join(err, ErrRecovery)) != 1 {
				t.Fatal("diagnostic propagation changed retirement safety")
			}
		})
	}
	synctest.Test(t, func(t *testing.T) {
		private := errors.New("SECRET/PIN/path")
		if observed := watchSourcePresence(context.Background(), func() error { return private }); observed != ErrAuthority {
			t.Fatal("watcher retained arbitrary callback details", observed)
		}
	})
}
