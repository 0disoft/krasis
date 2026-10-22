//go:build windows

package inputworker

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"os"
	"os/exec"
	"testing"
	"unsafe"

	"github.com/0disoft/krasis/internal/crashguard"
	"golang.org/x/sys/windows"
)

// Ordinary-user process fixture only. It accepts four event handles instead
// of authority/image pins and serves only the existing fake input port.
func TestPreparedNativeChildHelper(t *testing.T) {
	if os.Getenv("KRASIS_PREPARED_CHILD_FIXTURE") != "1" {
		return
	}
	var data [nativeBootstrapSize]byte
	if _, err := io.ReadFull(os.Stdin, data[:]); err != nil {
		os.Exit(2)
	}
	values := []uint64{binary.BigEndian.Uint64(data[8:16]), binary.BigEndian.Uint64(data[16:24]), binary.BigEndian.Uint64(data[bootstrapSize : bootstrapSize+8]), binary.BigEndian.Uint64(data[bootstrapSize+8:])}
	handles, err := adoptNativeHandles(values)
	if err != nil || data[24] != 93 {
		os.Exit(3)
	}
	for _, handle := range handles {
		state, err := windows.WaitForSingleObject(handle, 0)
		if err != nil || state != windows.WAIT_OBJECT_0 {
			os.Exit(4)
		}
		_ = windows.CloseHandle(handle)
	}
	input := &portableFixture{}
	_, err = crashguard.ServeWithInput(os.Stdin, os.Stdout, input, PortableInputHandler(input, func(_ context.Context, work func() error) error { return work() }))
	if err != nil {
		os.Exit(5)
	}
	os.Exit(0)
}

func TestPreparedNativePairTransfersExactHandlesOnlyAtBinding(t *testing.T) {
	pair := &preparedNativePair{}
	t.Cleanup(func() { _ = pair.Close() })
	for i := range pair.children {
		command := exec.Command(os.Args[0], "-test.run=^TestPreparedNativeChildHelper$")
		command.Env = append(os.Environ(), "KRASIS_PREPARED_CHILD_FIXTURE=1")
		owner, err := crashguard.PrepareLocalInputCommandContext(context.Background(), command)
		if err != nil {
			t.Fatal(err)
		}
		pair.children[i] = &preparedNativeChild{owner: owner, process: command.Process}
	}
	var handles []windows.Handle
	for range 4 {
		handle, err := windows.CreateEvent(nil, 1, 1, nil)
		if err != nil {
			t.Fatal(err)
		}
		defer windows.CloseHandle(handle)
		handles = append(handles, handle)
	}
	var bootstrap [nativeBootstrapSize]byte
	bootstrap[24] = 93
	input, err := startNativePair(context.Background(), func(ctx context.Context, release bool) (*isolatedInput, error) {
		return pair.bind(ctx, release, handles, bootstrap[:])
	}, nil)
	if err != nil {
		t.Fatal("target-handle bootstrap or readiness failed", err)
	}
	defer input.finishRecovery()
	defer input.retireForRecovery()
	if _, err := pair.bind(context.Background(), false, handles, bootstrap[:]); err == nil {
		t.Fatal("prepared child accepted a second binding")
	}
	if err := pair.Close(); err != nil || input.Heartbeat(context.Background()) != nil {
		t.Fatal("preparation retained ownership after transfer", err)
	}
	if retired, err := input.retireForRecovery(); !retired || err != nil {
		t.Fatal("bound input owner did not retire", retired, err)
	}
	if err := input.finishRecovery(); err != nil {
		t.Fatal("bound release owner did not retire", err)
	}
}

func TestPreparedInputConsoleUsesFixedDesktopAndBoundedJob(t *testing.T) {
	arguments, desktop, err := consoleLaunchMode(preparedInputConsole)
	if err != nil || arguments != " --prepared-input-worker" || desktop != `winsta0\default` {
		t.Fatal("preparation changed fixed process/desktop policy", arguments, desktop, err)
	}
	job, err := makeConsoleWorkerJob(preparedInputConsole)
	if err != nil {
		t.Fatal(err)
	}
	defer windows.CloseHandle(job)
	var limits windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION
	if err := windows.QueryInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits)), nil); err != nil {
		t.Fatal(err)
	}
	if limits.BasicLimitInformation.ActiveProcessLimit != 3 || limits.BasicLimitInformation.LimitFlags&windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE == 0 {
		t.Fatal("preparation escaped existing three-process ownership")
	}
	if !errors.Is(RunInstalledPreparedWorker(context.Background(), nil), ErrAuthority) {
		t.Fatal("prepared mode accepted a missing installation")
	}
}

func TestPreparedConsoleRequiresExactUnboundRetirementReceipt(t *testing.T) {
	for _, valid := range []bool{false, true} {
		server, client := nativePair(t)
		defer client.Close()
		worker := &ConsoleWorker{job: 11, process: 22, pairJob: true, nativeChildPossible: true, done: make(chan struct{}), closeOps: &consoleCloseOps{
			wait:            func(windows.Handle, uint32) (uint32, error) { return windows.WAIT_OBJECT_0, nil },
			closeHandle:     func(windows.Handle) error { return nil },
			terminateJob:    func(windows.Handle, uint32) error { return nil },
			activeProcesses: func(windows.Handle) (uint32, error) { return 0, nil },
		}}
		p := &preparedConsole{worker: worker, control: server, nativePrepared: true}
		t.Cleanup(func() {
			// The failed fixture has fake handles; retain no global retirement
			// fence that could affect unrelated real-process tests.
			consoleRetirements.Lock()
			delete(consoleRetirements.pending, worker)
			consoleRetirements.Unlock()
		})
		done := make(chan error, 1)
		go func() {
			if err := writeFrame(client, workerAuthenticatedFrame[:]); err != nil {
				done <- err
				return
			}
			var stop [bootstrapSize]byte
			if _, err := io.ReadFull(client, stop[:]); err != nil || !zeroBytes(stop[:]) {
				done <- ErrProtocol
				return
			}
			receipt := unboundRetiredFrame
			if !valid {
				receipt[4]++
			}
			done <- writeFrame(client, receipt[:])
		}()
		err := p.close()
		if (err == nil) != valid || worker.nativeChildRetired != valid || worker.closed != valid {
			t.Fatal("invalid receipt or accounting alone accepted as retirement", valid, err)
		}
		if err := <-done; err != nil {
			t.Fatal(err)
		}
		if (p.close() == nil) != valid {
			t.Fatal("cleanup failure lost on repeated close")
		}
	}
}
