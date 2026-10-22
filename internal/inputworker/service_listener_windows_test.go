//go:build windows

package inputworker

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/logindiag"
	"golang.org/x/sys/windows"
)

func testServiceListener(t *testing.T) (*servicePipeListener, string) {
	t.Helper()
	name := fmt.Sprintf(`\\.\pipe\Krasis.Listener.Test.%d.%s`, os.Getpid(), t.Name())
	listener, err := newServicePipeListener(name, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := listener.Close(); err != nil {
			t.Error(err)
		}
	})
	return listener, name
}

func TestServiceListenerAcceptSurvivesIdleStartupTimeout(t *testing.T) {
	listener, name := testServiceListener(t)
	parent, stop := context.WithTimeout(context.Background(), 10*time.Second)
	defer stop()
	server, err := listener.borrow()
	if err != nil {
		t.Fatal(err)
	}
	accept, cancel := agentAcceptContext(parent, true)
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- server.connect(accept) }()
	joined := false
	defer func() {
		cancel()
		if !joined {
			<-done
		}
	}()
	// Real kernel I/O: the old accept deadline retired this loan after five
	// seconds, before any client existed, even while the service remained live.
	timer := time.NewTimer(5*time.Second + 50*time.Millisecond)
	defer timer.Stop()
	select {
	case err := <-done:
		joined = true
		t.Fatal("idle service retired its listening instance", err)
	case <-timer.C:
	}
	client, err := openPipe(parent, name)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := <-done; err != nil {
		joined = true
		t.Fatal(err)
	}
	joined = true
	_ = client.SetDeadline(time.Now().Add(time.Second))
	_ = server.SetDeadline(time.Now().Add(time.Second))
	if _, err := client.Write([]byte("new")); err != nil {
		t.Fatal(err)
	}
	var data [3]byte
	if _, err := io.ReadFull(server, data[:]); err != nil || string(data[:]) != "new" {
		t.Fatal("client was reset after idle", err)
	}
	if err := server.Close(); err != nil {
		t.Fatal(err)
	}
	second, err := listener.borrow()
	if err != nil {
		t.Fatal(err)
	}
	shutdown := make(chan error, 1)
	go func() { shutdown <- second.connect(accept) }()
	cancel()
	if err := <-shutdown; !errors.Is(err, context.Canceled) {
		t.Fatal("service shutdown did not retire accept", err)
	}
}

func TestServiceListenerKeepsNameAcrossFailedOrCancelledLeaseBackoff(t *testing.T) {
	for _, mode := range []string{"input", "source"} {
		t.Run(mode, func(t *testing.T) {
			listener, name := testServiceListener(t)
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
			defer cancel()
			first, err := listener.borrow()
			if err != nil {
				t.Fatal(err)
			}
			oldClient, err := openPipe(ctx, name)
			if err != nil {
				t.Fatal(err)
			}
			defer oldClient.Close()
			if err := first.connect(ctx); err != nil {
				t.Fatal(err)
			}
			if _, err := oldClient.Write([]byte("old")); err != nil {
				t.Fatal(err)
			}
			if err := first.Close(); err != nil {
				t.Fatal(err)
			}

			// Simulate native recovery/optional construction after a cancelled
			// presentation. It lasts longer than input's unchanged 250ms grace.
			// The instance must remain bound instead of reporting missing-name.
			type outcome struct {
				pipe *pipeConn
				err  error
			}
			done := make(chan outcome, 1)
			started := make(chan struct{})
			go func() {
				close(started)
				pipe, err := openPipeWaiting(ctx, name, servicePipeMissingGrace)
				done <- outcome{pipe, err}
			}()
			joined := false
			defer func() {
				cancel()
				if !joined {
					if result := <-done; result.pipe != nil {
						_ = result.pipe.Close()
					}
				}
			}()
			<-started
			time.Sleep(350 * time.Millisecond)
			select {
			case result := <-done:
				joined = true
				if result.pipe != nil {
					_ = result.pipe.Close()
				}
				t.Fatalf("endpoint lost while recovery was joined: %v", result.err)
			default:
			}
			second, err := listener.borrow()
			if err != nil {
				t.Fatal(err)
			}
			connected := make(chan error, 1)
			go func() { connected <- second.connect(ctx) }()
			result := <-done
			joined = true
			if result.err != nil {
				t.Fatal(result.err)
			}
			defer result.pipe.Close()
			if err := <-connected; err != nil {
				t.Fatal(err)
			}
			if _, err := oldClient.Write([]byte("bad")); err == nil {
				t.Fatal("retired client could write into a later lease")
			}
			var pid uint32
			if windows.GetNamedPipeServerProcessId(result.pipe.handle, &pid) != nil || pid != windows.GetCurrentProcessId() {
				t.Fatal("listener lost the actual kernel owner")
			}
			if _, err := result.pipe.Write([]byte("new")); err != nil {
				t.Fatal(err)
			}
			_ = second.SetReadDeadline(time.Now().Add(time.Second))
			var data [3]byte
			if _, err := io.ReadFull(second, data[:]); err != nil || string(data[:]) != "new" {
				t.Fatal("retired client's unread bytes crossed the reset", data, err)
			}
		})
	}
}

func TestServiceListenerDoesNotResetAnOpenOwnerAndClosesEveryHandle(t *testing.T) {
	listener, name := testServiceListener(t)
	loan, err := listener.borrow()
	if err != nil {
		t.Fatal(err)
	}
	if second, err := listener.borrow(); second != nil || !errors.Is(err, ErrRecovery) {
		t.Fatal("open channel was disconnected/reassigned", err)
	}
	if _, err := makePipe(name); err == nil {
		t.Fatal("exclusive name ownership was lost")
	}
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-loan.closeDone:
	default:
		t.Fatal("loan remained open")
	}
	if next, err := listener.borrow(); next != nil || !errors.Is(err, ErrRecovery) {
		t.Fatal("closed listener reused", err)
	}
	fresh, err := makePipe(name)
	if err != nil {
		t.Fatal("exclusive endpoint leaked after shutdown", err)
	}
	_ = fresh.Close()
}

func TestServiceListenerReusesInstanceAfterCancelledUnconnectedAccept(t *testing.T) {
	listener, name := testServiceListener(t)
	first, err := listener.borrow()
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := first.connect(ctx); !errors.Is(err, context.Canceled) {
		t.Fatal("accept ignored cancellation", err)
	}
	if err := first.Close(); err != nil {
		t.Fatal(err)
	}
	next, err := listener.borrow()
	if err != nil {
		t.Fatal("unconnected reset failed", err)
	}
	// Allow OS scheduling under concurrent builds; this checks instance reuse
	// after cancellation, with no sub-second connection performance assertion.
	run, stop := context.WithTimeout(context.Background(), 5*time.Second)
	defer stop()
	connected := make(chan error, 1)
	go func() { connected <- next.connect(run) }()
	client, err := openPipe(run, name)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()
	if err := <-connected; err != nil {
		t.Fatal(err)
	}
}

func TestServiceListenerRetainsNativeFailureAndFencesUncertainClose(t *testing.T) {
	for _, stage := range []string{"duplicate", "reset", "close"} {
		t.Run(stage, func(t *testing.T) {
			name := fmt.Sprintf(`\\.\pipe\Krasis.Listener.Test.%d.%s`, os.Getpid(), t.Name())
			listener, err := newServicePipeListener(name, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer listener.Close() // failures are deliberately injected below.
			if stage != "duplicate" {
				loan, err := listener.borrow()
				if err != nil {
					t.Fatal(err)
				}
				if err := loan.Close(); err != nil {
					t.Fatal(err)
				}
				if stage == "close" {
					listener.loan.closeErr = ErrAuthority
				}
			}
			if stage != "close" {
				if err := listener.pipe.Close(); err != nil {
					t.Fatal(err)
				}
			}
			_, err = listener.borrow()
			var failure *servicePipeFailure
			if !errors.Is(err, ErrRecovery) || !errors.As(err, &failure) || failure.LoginRuntimeOperation() != "service-pipe-"+stage || !logindiag.ValidRuntimeOperation(failure.LoginRuntimeOperation()) {
				t.Fatal("listener failure lost operation or recovery fence", err)
			}
			if stage != "close" && !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
				t.Fatal("native failure was discarded", err)
			}
			var recorded logindiag.Runtime
			recorder := newLoginRuntimeRecorder(func(value logindiag.Runtime) error { recorded = value; return nil })
			recorder.record("service", "FAILED", serviceEndpointFailure(err, "input"))
			if recorded.LastFailure == nil || recorded.LastFailure.Operation != "service-pipe-"+stage || recorded.LastFailure.Reason != "input" {
				t.Fatal("service receipt lost the listener failure", recorded.LastFailure)
			}
			if stage != "close" && (!recorded.LastFailure.Win32Known || recorded.LastFailure.Win32Error != uint32(windows.ERROR_INVALID_HANDLE)) {
				t.Fatal("service receipt lost the original native error", recorded.LastFailure)
			}
			// The injected close failure must remain unverified on final close.
			if stage == "close" {
				if !errors.Is(listener.Close(), ErrRecovery) {
					t.Fatal("uncertain close became success")
				}
			}
		})
	}
}

func TestServiceListenerRetainsActualNativeCloseFailureInReceipt(t *testing.T) {
	name := fmt.Sprintf(`\\.\pipe\Krasis.Listener.Test.%d.%s`, os.Getpid(), t.Name())
	listener, err := newServicePipeListener(name, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close() // The failed close is deliberate; retain the owner until cleanup.
	loan, err := listener.borrow()
	if err != nil {
		t.Fatal(err)
	}
	if err := windows.CloseHandle(loan.handle); err != nil {
		t.Fatal(err)
	}
	// Do not leave a freed numeric handle available for accidental reuse.
	loan.handle = 0
	if err := loan.Close(); !errors.Is(err, net.ErrClosed) || !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
		t.Fatal("native close failure was erased", err)
	}
	_, err = listener.borrow()
	if !errors.Is(err, ErrRecovery) {
		t.Fatal("failed close did not fence listener reuse", err)
	}
	var recorded logindiag.Runtime
	recorder := newLoginRuntimeRecorder(func(value logindiag.Runtime) error { recorded = value; return nil })
	recorder.record("service", "FAILED", serviceEndpointFailure(err, "input"))
	if recorded.LastFailure == nil || recorded.LastFailure.Operation != "service-pipe-close" || !recorded.LastFailure.Win32Known || recorded.LastFailure.Win32Error != uint32(windows.ERROR_INVALID_HANDLE) {
		t.Fatal("service receipt lost the actual native close failure", recorded.LastFailure)
	}
	if err := listener.Close(); !errors.Is(err, windows.ERROR_INVALID_HANDLE) {
		t.Fatal("final listener close lost the original failure", err)
	}
}
