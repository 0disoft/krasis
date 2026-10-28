package inputworker

import (
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"testing/iotest"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/handoff"
)

func TestInputStartupFinalReportRejectsLeaseWithoutLosingRecovery(t *testing.T) {
	for _, cause := range []error{nil, ErrAuthority, ErrRecovery} {
		var wire bytes.Buffer
		if err := writeReport(&wire, Report{}, cause); err != nil {
			t.Fatal(err)
		}
		if _, err := readStartupReady(bytes.NewReader(wire.Bytes())); !errors.Is(err, ErrProtocol) {
			t.Fatal("legacy READY-only parser did not reproduce final-report rejection", err)
		}
		_, _, finished, err := readInputWorkerStartup(iotest.OneByteReader(bytes.NewReader(wire.Bytes())))
		if !finished || errors.Is(err, ErrProtocol) || cause != nil && !errors.Is(err, cause) {
			t.Fatal(finished, err)
		}
		_, err = readInputLeaseReady(bytes.NewReader(wire.Bytes()))
		if errors.Is(err, ErrStartupRetired) == errors.Is(cause, ErrRecovery) {
			t.Fatal("startup retirement lost cleanup certainty", cause, err)
		}
	}
}

func TestInputStartupRefusesWrongModeMutationAndIncompleteReports(t *testing.T) {
	exit := emptySourceExit()
	for _, report := range []Report{{Source: &exit}, {Completed: 1}, {Recovery: application.ReleaseReport{Attempted: 1, Released: 1}}} {
		var wire bytes.Buffer
		if err := writeReport(&wire, report, ErrAuthority); err != nil {
			t.Fatal(err)
		}
		if _, err := readInputLeaseReady(&wire); !errors.Is(err, ErrRecovery) || errors.Is(err, ErrStartupRetired) {
			t.Fatal(err)
		}
	}
	var report bytes.Buffer
	_ = writeReport(&report, Report{}, ErrAuthority)
	for size := 0; size < report.Len(); size++ {
		if _, err := readInputLeaseReady(bytes.NewReader(report.Bytes()[:size])); err == nil || errors.Is(err, ErrStartupRetired) {
			t.Fatal(size, err)
		}
	}
}

func TestInputStartupReadyDoesNotConsumeLaterReport(t *testing.T) {
	var wire bytes.Buffer
	_ = writeStartupReady(&wire, handoff.StartupProfile{})
	_ = writeReport(&wire, Report{}, nil)
	if _, err := readInputLeaseReady(&wire); err != nil {
		t.Fatal(err)
	}
	if _, err := readReport(&wire); err != nil || wire.Len() != 0 {
		t.Fatal("READY consumed final report", err)
	}
}

func TestInputFinalReportWaitsForNativeRetirementAndPreservesFailure(t *testing.T) {
	for _, retirementErr := range []error{nil, ErrAuthority} {
		server, child := net.Pipe()
		entered, release := make(chan struct{}), make(chan struct{})
		done := make(chan error, 1)
		go func() {
			done <- finishInputWorkerReport(child, Report{}, ErrAuthority, func() error { close(entered); <-release; return retirementErr })
			_ = child.Close()
		}()
		<-entered
		close(release)
		_, err := readInputLeaseReady(server)
		result := <-done
		_ = server.Close()
		if !errors.Is(result, ErrAuthority) || errors.Is(err, ErrStartupRetired) != (retirementErr == nil) || errors.Is(err, ErrRecovery) != (retirementErr != nil) {
			t.Fatal("native retirement failure became a clean startup rejection", result, err)
		}
	}
}

func TestStartupRetiredPublisherCannotForwardUnknownCleanup(t *testing.T) {
	for _, cause := range []error{ErrRecovery, ErrProtocol} {
		stream := &startupPublishFixture{}
		if err := publishInputStartupRetired(context.Background(), stream, Report{}, cause); err == nil || stream.Len() != 0 {
			t.Fatal("unsafe retirement published", err)
		}
	}
}
