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

func TestSupervisorMissingReceiptIsRecoveryUnknown(t *testing.T) {
	server, client := net.Pipe()
	_ = server.Close()
	_, err := supervise(context.Background(), client, func() error { return nil }, func() bool { return true }, time.Now().Add(time.Minute))
	if err != ErrRecovery {
		t.Fatal("worker exit treated as successful recovery")
	}
}

func TestSupervisorRejectsReceiptWithRemainingInput(t *testing.T) {
	server, client := net.Pipe()
	go func() {
		defer server.Close()
		_ = writeReport(server, Report{Recovery: application.ReleaseReport{Remaining: 1}}, ErrRecovery)
	}()
	report, err := supervise(context.Background(), client, func() error { return nil }, func() bool { return false }, time.Now().Add(time.Minute))
	if err != ErrRecovery || report.Recovery.Remaining != 1 {
		t.Fatal("failed recovery hidden")
	}
}
