package inputworker

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

type supervisorControlSpy struct {
	net.Conn
	writes, readDeadlines, writeDeadlines int
}

func (s *supervisorControlSpy) Write(p []byte) (int, error)      { s.writes++; return len(p), nil }
func (s *supervisorControlSpy) SetReadDeadline(time.Time) error  { s.readDeadlines++; return nil }
func (s *supervisorControlSpy) SetWriteDeadline(time.Time) error { s.writeDeadlines++; return nil }

func TestSupervisorPublishedReceiptPrecedesReadyPollAndCancellation(t *testing.T) {
	for _, cancelled := range []bool{false, true} {
		for _, failure := range []error{nil, ErrAuthority, ErrProtocol, ErrRecovery} {
			ctx, cancel := context.WithCancel(context.Background())
			if cancelled {
				cancel()
			}
			server, client := net.Pipe()
			control := &supervisorControlSpy{Conn: client}
			results := make(chan workerReportResult, 1)
			results <- workerReportResult{report: Report{Completed: 3}, err: failure}
			polls := make(chan time.Time, 1)
			polls <- time.Now()
			calls := 0
			report, err := superviseWorkerResults(ctx, control, func() error { calls++; return ErrAuthority }, func() bool { calls++; return false }, time.Now().Add(time.Hour), results, polls)
			cancel()
			_ = server.Close()
			_ = client.Close()
			if calls != 0 || control.writes != 0 || control.readDeadlines != 0 || control.writeDeadlines != 0 || errors.Is(err, ErrRecovery) != (failure == ErrRecovery || failure == ErrProtocol) || errors.Is(err, ErrProtocol) != (failure == ErrProtocol) || (err == nil) != (failure == nil) {
				t.Fatalf("ready receipt changed or lost priority: cancelled=%v cause=%v err=%v calls=%d", cancelled, failure, err, calls)
			}
			if failure != ErrProtocol && report.Completed != 3 {
				t.Fatal("completed receipt lost", report)
			}
		}
	}
}

func TestSupervisorReceiptDuringPolicyQueryPrecedesStopWrite(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()
	control := &supervisorControlSpy{Conn: client}
	results := make(chan workerReportResult, 1)
	polls := make(chan time.Time, 1)
	polls <- time.Now()
	checked := 0
	report, err := superviseWorkerResults(context.Background(), control, func() error {
		checked++
		results <- workerReportResult{report: Report{Recovery: application.ReleaseReport{Attempted: 1, Remaining: 1}}}
		return ErrAuthority
	}, func() bool { checked++; return false }, time.Now().Add(time.Hour), results, polls)
	if !errors.Is(err, ErrRecovery) || report.Recovery.Remaining != 1 || checked != 2 || control.writes != 0 || control.readDeadlines != 0 || control.writeDeadlines != 0 {
		t.Fatal("completed uncertain recovery was delayed or lost", report, err, checked)
	}
}
