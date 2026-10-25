package inputworker

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"syscall"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/crashguard"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestPreparationFailureReachesServiceWithoutCertifyingRetirement(t *testing.T) {
	for _, tc := range []struct {
		phase     byte
		cause     error
		operation string
		code      uint32
	}{
		{preparationBootstrapFailure, context.DeadlineExceeded, "bootstrap-timeout", 0},
		{preparationBootstrapFailure, context.Canceled, "bootstrap-cancelled", 0},
		{preparationBootstrapFailure, io.ErrUnexpectedEOF, "bootstrap-truncated", 0},
		{preparationBootstrapFailure, io.EOF, "bootstrap-eof", 0},
		{preparationNativeFailure, net.ErrClosed, "native-closed", 0},
		{preparationNativeFailure, errors.New("private child stderr"), "native-other", 0},
		{preparationNativeFailure, syscall.Errno(5), "native-win32", 5},
		{preparationNativeFailure, &crashguard.ExitPending{Cause: errors.Join(crashguard.ErrExitTimeout, syscall.Errno(6))}, "native-pending", 6},
	} {
		t.Run(tc.operation, func(t *testing.T) {
			frame := preparationFailureFrame(tc.phase, tc.cause)
			stream := &inputRetirementFixture{Reader: io.MultiReader(bytes.NewReader(workerAuthenticatedFrame[:]), bytes.NewReader(frame[:]))}
			err := retireInputPreparation(stream, workerAuthenticatedFrame, unboundRetiredFrame)
			if !errors.Is(err, ErrRecovery) {
				t.Fatal("negative diagnostic report certified retirement", err)
			}
			recorder := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
			recorder.record("service", "FAILED", serviceEndpointFailure(err, "input"))
			failure := recorder.value.LastFailure
			if failure == nil || failure.Operation != "input-retire-"+tc.operation || failure.Win32Error != tc.code || failure.Win32Known != (tc.code != 0) || !recorder.value.Valid() {
				t.Fatal("child failure was lost or private text was exported", failure)
			}
		})
	}
}

func TestPreparationFailureRejectsMalformedAndTruncatedReports(t *testing.T) {
	frame := preparationFailureFrame(preparationNativeFailure, syscall.Errno(5))
	for i, value := range map[int]byte{0: 'X', 4: 0, 5: 8, 6: 2, 7: 1, 11: 0} {
		invalid := frame
		invalid[i] = value
		if err := readPreparationRetirement(bytes.NewReader(invalid[:]), unboundRetiredFrame); !errors.Is(err, ErrProtocol) {
			t.Fatal("malformed diagnostic report accepted", i, err)
		}
	}
	for n := 0; n < len(frame); n++ {
		if err := readPreparationRetirement(bytes.NewReader(frame[:n]), unboundRetiredFrame); !errors.Is(err, ErrProtocol) {
			t.Fatal("truncated diagnostic report accepted", n, err)
		}
	}
	if err := readInputServiceFrame(bytes.NewReader(frame[:]), workerAuthenticatedFrame); !errors.Is(err, ErrProtocol) {
		t.Fatal("negative report substituted for worker authentication", err)
	}
}

func TestPreparationFailureWriteIsBoundedAndKeepsOriginalCause(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		service, worker := net.Pipe()
		defer service.Close()
		defer worker.Close()
		started := time.Now()
		cause := syscall.Errno(5)
		err := reportPreparationFailure(worker, preparationNativeFailure, cause)
		if !errors.Is(err, cause) || time.Since(started) != 250*time.Millisecond {
			t.Fatal("unread diagnostic delayed cleanup or lost original failure", err)
		}
	})
}
