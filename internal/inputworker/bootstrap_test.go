package inputworker

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputdesktop"
)

func TestLeaseBootstrapRejectsPaddingExpiredAndUnboundedLeases(t *testing.T) {
	now := time.Now()
	lease := testLease()
	data, err := encodeLease(lease, now)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := decodeLease(data, now)
	if err != nil || decoded.Identity != lease.Identity || decoded.Binding != lease.Binding || decoded.ExpiresAt.UnixMilli() != lease.ExpiresAt.UnixMilli() {
		t.Fatal("lease roundtrip changed authority")
	}
	for _, corrupt := range []func(*[leaseSize]byte){
		func(b *[leaseSize]byte) { b[424] = 1 },
		func(b *[leaseSize]byte) { b[38] = 1 },
		func(b *[leaseSize]byte) { b[167] = 1 },
		func(b *[leaseSize]byte) {
			binary.BigEndian.PutUint64(b[8:16], uint64(now.Add(-time.Second).UnixMilli()))
		},
		func(b *[leaseSize]byte) {
			binary.BigEndian.PutUint64(b[8:16], uint64(now.Add(2*maxLeaseDuration).UnixMilli()))
		},
		func(b *[leaseSize]byte) { binary.BigEndian.PutUint16(b[32:34], 129) },
	} {
		bad := data
		corrupt(&bad)
		if _, err := decodeLease(bad, now); err == nil {
			t.Fatal("malformed lease accepted")
		}
	}
	lease.Identity.Epoch = "bad\x00epoch"
	if _, err := encodeLease(lease, now); err == nil {
		t.Fatal("noncanonical identity accepted")
	}
}

func TestWorkerReportSeparatesRecoveryFailureAndProcessCompletion(t *testing.T) {
	for _, result := range []struct {
		report Report
		err    error
	}{
		{Report{Completed: 3, Recovery: application.ReleaseReport{Attempted: 2, Released: 2}}, nil},
		{Report{Recovery: application.ReleaseReport{Attempted: 2, Released: 1, Remaining: 1}}, ErrRecovery},
		{Report{}, ErrAuthority},
	} {
		var buffer bytes.Buffer
		if writeReport(&buffer, result.report, result.err) != nil {
			t.Fatal("valid worker receipt rejected")
		}
		actual, err := readReport(&buffer)
		if actual != result.report || !errors.Is(err, result.err) {
			t.Fatal("receipt result changed")
		}
	}
	var buffer bytes.Buffer
	_ = writeReport(&buffer, Report{}, nil)
	bad := buffer.Bytes()
	binary.BigEndian.PutUint32(bad[24:28], 1)
	if _, err := readReport(bytes.NewReader(bad)); err != ErrProtocol {
		t.Fatal("success with stuck input accepted")
	}
}

func TestSupervisorWaitsForRecoveryReceiptBeforeReturning(t *testing.T) {
	server, client := net.Pipe()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	stopSeen := make(chan struct{})
	released := make(chan struct{})
	go func() {
		defer server.Close()
		var command [1]byte
		_, _ = io.ReadFull(server, command[:])
		close(stopSeen)
		<-released
		_ = writeReport(server, Report{Recovery: application.ReleaseReport{Attempted: 2, Released: 2}}, nil)
	}()
	done := make(chan error, 1)
	go func() {
		_, err := supervise(ctx, client, func() error { return nil }, func() bool { return false }, time.Now().Add(time.Minute))
		done <- err
	}()
	<-stopSeen
	select {
	case <-done:
		t.Fatal("shutdown returned before recovery receipt")
	default:
	}
	close(released)
	select {
	case err := <-done:
		if err != nil {
			t.Fatal("completed recovery rejected")
		}
	case <-time.After(time.Second):
		t.Fatal("completed recovery remained blocked")
	}
}

func TestWorkerStopPreservesDesktopCleanupAndRecoveryReceipt(t *testing.T) {
	for _, cause := range []error{inputdesktop.ErrCleanup, errors.Join(context.Canceled, inputdesktop.ErrCleanup)} {
		result := normalizeWorkerStop(true, Report{}, cause)
		if !errors.Is(result, inputdesktop.ErrCleanup) {
			t.Fatal("empty journal hid desktop cleanup failure")
		}
		var wire bytes.Buffer
		if err := writeReport(&wire, Report{}, result); err != nil {
			t.Fatal(err)
		}
		if _, err := readReport(&wire); !errors.Is(err, ErrRecovery) {
			t.Fatal("cleanup failure did not fence the next lease", err)
		}
	}
	if normalizeWorkerStop(true, Report{}, errors.Join(context.Canceled, context.Canceled)) != nil {
		t.Fatal("pure requested cancellation not normalized")
	}
	for _, tc := range []struct {
		requested bool
		report    Report
		cause     error
	}{
		{false, Report{}, context.Canceled},
		{true, Report{Recovery: application.ReleaseReport{Remaining: 1}}, context.Canceled},
		{true, Report{}, errors.Join(context.Canceled, ErrAuthority)},
		{true, Report{}, errors.Join(context.Canceled, context.DeadlineExceeded)},
	} {
		if normalizeWorkerStop(tc.requested, tc.report, tc.cause) == nil {
			t.Fatal("non-clean stop normalized")
		}
	}
}

func TestSupervisorMissingReceiptIsRecoveryUnknown(t *testing.T) {
	server, client := net.Pipe()
	_ = server.Close()
	_, err := supervise(context.Background(), client, func() error { return nil }, func() bool { return true }, time.Now().Add(time.Minute))
	if !errors.Is(err, ErrRecovery) || !errors.Is(err, ErrProtocol) {
		t.Fatal("worker exit treated as successful recovery")
	}
	var detail *workerReportFailure
	if !errors.As(err, &detail) || detail.operation != "worker-report-eof" {
		t.Fatal("missing receipt lost its failure operation", err)
	}
}

func TestSupervisorRejectsReceiptWithRemainingInput(t *testing.T) {
	server, client := net.Pipe()
	go func() {
		defer server.Close()
		_ = writeReport(server, Report{Recovery: application.ReleaseReport{Remaining: 1}}, ErrRecovery)
	}()
	report, err := supervise(context.Background(), client, func() error { return nil }, func() bool { return false }, time.Now().Add(time.Minute))
	if !errors.Is(err, ErrRecovery) || report.Recovery.Remaining != 1 {
		t.Fatal("failed recovery hidden")
	}
}
