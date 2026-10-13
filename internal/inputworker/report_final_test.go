package inputworker

import (
	"errors"
	"net"
	"syscall"
	"testing"
	"time"
)

func TestFinalWorkerReportRenewsExpiredStartupDeadline(t *testing.T) {
	for _, failure := range []error{nil, errors.Join(ErrRecovery, syscall.Errno(5))} {
		child, service := net.Pipe()
		// Deterministically simulate a capture owner that outlived readiness's
		// write deadline. No sleeping, desktop access or process launch is needed.
		_ = child.SetWriteDeadline(time.Now().Add(-time.Second))
		if err := writeReport(child, Report{}, failure); err == nil {
			t.Fatal("test did not reproduce expired capture report")
		}
		_ = service.SetReadDeadline(time.Now().Add(time.Second))
		done := make(chan error, 1)
		go func() { done <- finishWorkerReport(child, Report{}, failure); _ = child.Close() }()
		_, received := readReport(service)
		returned := <-done
		_ = service.Close()
		if failure == nil {
			if returned != nil || received != nil {
				t.Fatal("clean long-lived capture became uncertain recovery", returned, received)
			}
		} else if !errors.Is(received, ErrRecovery) || !errors.Is(received, syscall.Errno(5)) || !errors.Is(returned, failure) {
			t.Fatal("actual recovery/native failure was lost", returned, received)
		}
	}
}

func TestFinalWorkerReportCannotClaimCleanExitWithoutDelivery(t *testing.T) {
	child, service := net.Pipe()
	_ = service.Close()
	defer child.Close()
	if err := finishWorkerReport(child, Report{}, nil); !errors.Is(err, ErrRecovery) {
		t.Fatal("missing report was accepted", err)
	}
	failure := syscall.Errno(5)
	if err := finishWorkerReport(child, Report{}, failure); !errors.Is(err, failure) || !errors.Is(err, ErrRecovery) {
		t.Fatal("delivery failure erased original cause", err)
	}
}
