//go:build windows

package inputworker

import (
	"context"
	"errors"
	"syscall"
	"testing"

	"golang.org/x/sys/windows"
)

func TestInputPreparationLaunchKeepsNativeCauseAndSourceStartupStage(t *testing.T) {
	prepared, err := prepareConsole(context.Background(), func(func(windows.Handle, uint32) error) (*ConsoleWorker, error) {
		return nil, errors.Join(ErrAuthority, syscall.Errno(5))
	})
	var detail *inputServiceFailure
	if prepared != nil || !errors.Is(err, ErrAuthority) || !errors.Is(err, syscall.Errno(5)) || !errors.As(err, &detail) || detail.operation != "input-prepare-launch" {
		t.Fatal("launch lost its phase or native cause", err)
	}
	source := sourceStartupFailure("prepare", Report{}, err)
	var operation interface{ LoginRuntimeOperation() string }
	if !errors.As(source, &operation) || operation.LoginRuntimeOperation() != "source-startup-prepare" || !errors.Is(source, syscall.Errno(5)) {
		t.Fatal("input preparation changed source startup semantics", source)
	}
}

func TestInputStartupRetainsCandidateCleanupFailure(t *testing.T) {
	candidate := &preparedConsole{worker: failedConsoleCloseForTest()}
	_, result := runPreparedAgentLease(nil, nil, Lease{}, [32]byte{}, [32]byte{}, nil, candidate)
	var detail *inputServiceFailure
	if !errors.Is(result, ErrRecovery) || !errors.Is(result, ErrAuthority) || !errors.As(result, &detail) || detail.operation != "input-close-worker" {
		t.Fatal("startup discarded owned cleanup stage or recovery fence", result)
	}
}

func TestNativeStartupCompletionRetainsWindowsFailure(t *testing.T) {
	// No I/O is submitted. An explicitly pending completion returns the real
	// Windows incomplete-I/O error without touching an installed process.
	pipe := &pipeConn{}
	_, err := pipe.overlapped(context.Background(), true, []byte{0}, func(overlap *windows.Overlapped) error {
		overlap.Internal = uintptr(windows.STATUS_PENDING)
		return nil
	})
	if !errors.Is(err, ErrProtocol) || !errors.Is(err, windows.ERROR_IO_INCOMPLETE) {
		t.Fatal("native completion discarded Windows error", err)
	}
}
