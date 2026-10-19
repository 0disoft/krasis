package inputworker

import (
	"bytes"
	"errors"
	"net"
	"testing"

	"github.com/0disoft/krasis/internal/handoff"
)

func TestSourceWorkerRetiresBeforeReadyWithoutLosingServiceReport(t *testing.T) {
	service, child := net.Pipe()
	defer service.Close()
	done := make(chan error, 1)
	exit := emptySourceExit()
	exit.Capture, exit.Sender, exit.Interrupt, exit.Retirement = "joined", "joined", "joined", "clean_sent"
	go func() {
		done <- finishWorkerReport(child, Report{Source: &exit}, nil)
		_ = child.Close()
	}()
	profile, report, finished, err := readSourceWorkerStartup(service)
	if err != nil || !finished || profile != nil || report.Source == nil || report.Source.Capture != "joined" || report.Source.Retirement != "clean_sent" {
		t.Fatal("early retirement discarded or became readiness", report, finished, err)
	}
	if err := <-done; err != nil {
		t.Fatal(err)
	}
}

func TestSourceWorkerStartupPreservesRecoveryAndNeverAcceptsOtherReports(t *testing.T) {
	exit := emptySourceExit()
	exit.Capture, exit.Retirement = "failed", "failed_sent"
	var failed bytes.Buffer
	if err := writeReport(&failed, Report{Source: &exit}, ErrRecovery); err != nil {
		t.Fatal(err)
	}
	profile, report, finished, err := readSourceWorkerStartup(bytes.NewReader(failed.Bytes()))
	if profile != nil || !finished || report.Source == nil || !errors.Is(err, ErrRecovery) {
		t.Fatal("cleanup failure lost", report, err)
	}
	for _, data := range [][]byte{nil, failed.Bytes()[:7], failed.Bytes()[:12], []byte("invalid!")} {
		profile, _, _, err := readSourceWorkerStartup(bytes.NewReader(data))
		if profile != nil || !errors.Is(err, ErrRecovery) {
			t.Fatal("missing or invalid source report passed cleanup fence", len(data), err)
		}
	}
}

func TestSourceWorkerStartupRejectsWrongModeRegardlessOfReportedOutcome(t *testing.T) {
	for _, failure := range []error{nil, ErrAuthority, ErrRecovery} {
		var wire bytes.Buffer
		if err := writeReport(&wire, Report{}, failure); err != nil {
			t.Fatal(err)
		}
		profile, report, finished, err := readSourceWorkerStartup(&wire)
		if profile != nil || report.Source != nil || !finished || !errors.Is(err, ErrRecovery) {
			t.Fatal("wrong-mode report escaped source cleanup fence", failure, err)
		}
		if failure != nil && !errors.Is(err, failure) {
			t.Fatal("wrong-mode rejection discarded reported failure", failure, err)
		}
	}
}

func TestSourceWorkerStartupReadyLeavesFollowingReportForSupervisor(t *testing.T) {
	var wire bytes.Buffer
	if err := writeStartupReady(&wire, handoff.StartupProfile{}); err != nil {
		t.Fatal(err)
	}
	exit := emptySourceExit()
	_ = writeReport(&wire, Report{Source: &exit}, nil)
	_, report, finished, err := readSourceWorkerStartup(&wire)
	if err != nil || finished || report.Source != nil {
		t.Fatal("normal startup consumed final report", err)
	}
	if report, err := readReport(&wire); err != nil || report.Source == nil || wire.Len() != 0 {
		t.Fatal("following report lost framing", err)
	}
}
