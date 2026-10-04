//go:build windows

package inputworker

import (
	"context"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	"golang.org/x/sys/windows"
)

func TestPipeDialWaitsOnlyBeforeConnectionAndPreservesBusyBehavior(t *testing.T) {
	marker := &pipeConn{}
	calls := 0
	pipe, err := dialPipe(context.Background(), 100*time.Millisecond, func() (*pipeConn, error) {
		calls++
		switch calls {
		case 1:
			return nil, windows.ERROR_FILE_NOT_FOUND
		case 2:
			return nil, windows.ERROR_PIPE_BUSY
		default:
			return marker, nil
		}
	})
	if err != nil || pipe != marker || calls != 3 {
		t.Fatal("temporary missing/busy endpoint did not recover")
	}
	calls = 0
	pipe, err = dialPipe(context.Background(), 0, func() (*pipeConn, error) {
		calls++
		if calls == 1 {
			return nil, windows.ERROR_PIPE_BUSY
		}
		return marker, nil
	})
	if err != nil || pipe != marker || calls != 2 {
		t.Fatal("generic busy-pipe behavior changed")
	}
}

func TestPipeDialMissingGraceIsBoundedAndKeepsNativeCause(t *testing.T) {
	started := time.Now()
	calls := 0
	pipe, err := dialPipe(context.Background(), 25*time.Millisecond, func() (*pipeConn, error) {
		calls++
		return nil, windows.ERROR_FILE_NOT_FOUND
	})
	if pipe != nil || err != windows.ERROR_FILE_NOT_FOUND || calls < 2 || time.Since(started) > time.Second {
		t.Fatal("missing grace reset its budget or lost the native error")
	}
	// A previous busy wait consumes the same missing-name budget, rather than
	// granting another grace period when the name vanishes at the next open.
	calls = 0
	pipe, err = dialPipe(context.Background(), time.Millisecond, func() (*pipeConn, error) {
		calls++
		if calls == 1 {
			return nil, windows.ERROR_PIPE_BUSY
		}
		return nil, windows.ERROR_FILE_NOT_FOUND
	})
	if pipe != nil || err != windows.ERROR_FILE_NOT_FOUND || calls != 2 {
		t.Fatal("busy-to-missing transition restarted grace")
	}
}

func TestPipeDialNeverRetriesDeniedInvalidOrBrokenEndpoints(t *testing.T) {
	for _, failure := range []error{windows.ERROR_ACCESS_DENIED, windows.ERROR_PATH_NOT_FOUND, windows.ERROR_BROKEN_PIPE, ErrAuthority, ErrProtocol} {
		calls := 0
		pipe, err := dialPipe(context.Background(), servicePipeMissingGrace, func() (*pipeConn, error) {
			calls++
			return nil, failure
		})
		if pipe != nil || err != failure || calls != 1 {
			t.Fatal("nontransient failure retried")
		}
	}
	calls := 0
	if _, err := dialPipe(context.Background(), 0, func() (*pipeConn, error) {
		calls++
		return nil, windows.ERROR_FILE_NOT_FOUND
	}); err != windows.ERROR_FILE_NOT_FOUND || calls != 1 {
		t.Fatal("worker control pipe inherited service-only missing grace")
	}
}

func TestPipeDialCancellationAndEarlierDeadlineEndWait(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	open := func() (*pipeConn, error) { calls++; return nil, windows.ERROR_FILE_NOT_FOUND }
	if _, err := dialPipe(ctx, servicePipeMissingGrace, open); !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("canceled dial opened a pipe")
	}
	ctx, cancel = context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := dialPipe(ctx, servicePipeMissingGrace, open); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatal("missing grace extended parent deadline")
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	if _, err := dialPipe(ctx, servicePipeMissingGrace, func() (*pipeConn, error) {
		cancel()
		return nil, windows.ERROR_FILE_NOT_FOUND
	}); !errors.Is(err, context.Canceled) {
		t.Fatal("cancel during missing-name wait was ignored")
	}
}

func TestNativeMissingPipeCanAppearDuringServiceGraceWithoutInput(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\Krasis.Input.Test.%d.%s`, os.Getpid(), t.Name())
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if client, err := openPipe(ctx, name); client != nil || err != windows.ERROR_FILE_NOT_FOUND {
		t.Fatal("fixture pipe unexpectedly exists or generic open waits")
	}
	type result struct {
		server *pipeConn
		err    error
	}
	done := make(chan result, 1)
	go func() {
		timer := time.NewTimer(30 * time.Millisecond)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			done <- result{err: ctx.Err()}
		case <-timer.C:
			server, err := makePipe(name)
			if err == nil {
				err = server.connect(ctx)
			}
			done <- result{server, err}
		}
	}()
	client, openErr := openPipeWaiting(ctx, name, servicePipeMissingGrace)
	if client != nil {
		defer client.Close()
	}
	if openErr != nil {
		cancel()
	}
	server := <-done
	if server.server != nil {
		defer server.server.Close()
	}
	if openErr != nil || server.err != nil || client == nil {
		t.Fatal("late native endpoint did not connect within grace")
	}
	var pid uint32
	if windows.GetNamedPipeServerProcessId(client.handle, &pid) != nil || pid != windows.GetCurrentProcessId() {
		t.Fatal("native pipe owner metadata unavailable after delayed open")
	}
}

func TestNativePipeOpenedDuringCancellationIsClosed(t *testing.T) {
	_, client := nativePair(t)
	ctx, cancel := context.WithCancel(context.Background())
	pipe, err := dialPipe(ctx, servicePipeMissingGrace, func() (*pipeConn, error) {
		cancel()
		return client, nil
	})
	if pipe != nil || err != context.Canceled {
		t.Fatal("open completion ignored cancellation")
	}
	select {
	case <-client.closeDone:
	default:
		t.Fatal("canceled successful open leaked native handle")
	}
}
