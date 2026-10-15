package inputworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"syscall"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/logindiag"
	"github.com/0disoft/krasis/internal/peer"
)

func cleanSourceExit() logindiag.SourceExit {
	return logindiag.SourceExit{Command: "ok", Reader: "stopped", Capture: "joined", Sender: "joined", Interrupt: "joined", Retirement: "clean_sent", Report: "ok"}
}

func TestSourceExitRoundTripRetainsCommandReaderJoinsAndSeparateReportStatus(t *testing.T) {
	for _, tc := range []struct {
		command, reader, sender, interrupt, retirement, report string
		failure                                                error
	}{
		{"ok", "stopped", "joined", "joined", "clean_sent", "ok", nil},
		{"inspection", "cancelled", "joined", "joined", "clean_sent", "failed", ErrAuthority},
		{"protocol", "invalid", "joined", "joined", "delivery_failed", "failed", ErrProtocol},
		{"cleanup", "eof", "unjoined", "joined", "failed_sent", "recovery", ErrRecovery},
		{"cancelled", "timeout", "joined", "unjoined", "delivery_failed", "recovery", ErrRecovery},
	} {
		exit := cleanSourceExit()
		exit.Command, exit.Reader, exit.Sender, exit.Interrupt, exit.Retirement = tc.command, tc.reader, tc.sender, tc.interrupt, tc.retirement
		var wire bytes.Buffer
		if err := writeReport(&wire, Report{Source: &exit}, tc.failure); err != nil {
			t.Fatal(err)
		}
		if wire.Len() != reportSize || wire.Bytes()[4] != 3 || wire.Bytes()[7] != 1 {
			t.Fatal("source report confused with legacy injection counters")
		}
		report, result := readReport(&wire)
		exit.Report = tc.report
		if report.Source == nil || *report.Source != exit || report.Completed != 0 || report.Recovery != (application.ReleaseReport{}) || errors.Is(result, ErrRecovery) != errors.Is(tc.failure, ErrRecovery) {
			t.Fatal("source evidence or recovery proof changed", report, result)
		}
	}
}

func TestSourceExitWireRejectsUnknownFieldsImpossibleSuccessAndInputCounters(t *testing.T) {
	exit := cleanSourceExit()
	var wire bytes.Buffer
	if err := writeReport(&wire, Report{Source: &exit}, nil); err != nil {
		t.Fatal(err)
	}
	for _, mutate := range []func([]byte){
		func(data []byte) { data[7] = 0 },
		func(data []byte) { data[8] = 255 },
		func(data []byte) { data[11] = 2 }, // An unjoined sender cannot carry success.
		func(data []byte) { data[14] = 1 },
		func(data []byte) { data[15] = 1 },
		func(data []byte) { data[19] = 1 }, // No aliasing source and injection fields.
	} {
		bad := append([]byte(nil), wire.Bytes()...)
		mutate(bad)
		if _, err := readReport(bytes.NewReader(bad)); err != ErrProtocol {
			t.Fatal("invalid source evidence accepted", bad, err)
		}
	}
	exit.Command = "PRIVATE/input"
	if err := writeReport(io.Discard, Report{Source: &exit}, ErrRecovery); err != ErrProtocol {
		t.Fatal("private source state accepted", err)
	}
	exit = cleanSourceExit()
	if err := writeReport(io.Discard, Report{Completed: 1, Source: &exit}, nil); err != ErrProtocol {
		t.Fatal("source observations aliased an input count")
	}
	if failure := validateSourceReportMode(false, Report{Source: &exit}, nil); !errors.Is(failure, ErrRecovery) || !errors.Is(failure, ErrProtocol) {
		t.Fatal("source summary was accepted as an injection recovery report", failure)
	}
	for status := byte(0); status < 3; status++ {
		for field, vocabulary := range sourceExitVocabulary {
			for code := range vocabulary {
				var data [8]byte
				data[field] = byte(code)
				if status != 2 && (field == 2 || field == 3 || field == 4) && code == 2 {
					continue
				}
				if _, err := decodeSourceExit(data[:], status); err != nil {
					t.Fatal("bounded source vocabulary drifted", status, field, code, err)
				}
			}
		}
	}
}

func TestSourceExitHistoryIsBoundedImmutableAndKeepsPriorFailure(t *testing.T) {
	var saved []logindiag.Runtime
	r := newLoginRuntimeRecorder(func(v logindiag.Runtime) error { saved = append(saved, v); return nil })
	r.record("service", "READY", nil)
	r.record("worker", "FAILED", syscall.Errno(5))
	exit := cleanSourceExit()
	var first *logindiag.SourceHistory
	for i := 0; i < logindiag.MaxSourceExits+3; i++ {
		r.recordSourceExit(Report{Source: &exit})
		if i == 0 {
			first = r.source.Load()
		}
	}
	if len(saved) != 2 || r.value.Source != nil {
		t.Fatal("source endpoint performed synchronous disk persistence")
	}
	r.record("console", "USER_SESSION", nil)
	if r.value.Source == nil || len(r.value.Source.Exits) != logindiag.MaxSourceExits || r.value.Source.Total != logindiag.MaxSourceExits+3 || r.value.LastFailure == nil || r.value.LastFailure.Win32Error != 5 || !r.value.Valid() {
		t.Fatal("source history lost failure or exceeded bound", r.value)
	}
	if first.Total != 1 || len(first.Exits) != 1 {
		t.Fatal("later exits mutated published history", first)
	}
	data, err := json.Marshal(r.value)
	if err != nil || len(data) > 16384 {
		t.Fatal("bounded history exceeds runtime reader cap", len(data), err)
	}
	count := len(saved)
	exit.Reader = "PRIVATE/input"
	r.recordSourceExit(Report{Source: &exit})
	if len(saved) != count {
		t.Fatal("unbounded source evidence persisted")
	}
	r.persist = func(logindiag.Runtime) error { return io.ErrClosedPipe }
	exit = cleanSourceExit()
	r.recordSourceExit(Report{Source: &exit})
	r.record("console", "USER_SESSION", nil)
	if r.value.Source.Total != logindiag.MaxSourceExits+3 {
		t.Fatal("failed persistence marked as saved")
	}
	if r.source.Load().Total != logindiag.MaxSourceExits+4 {
		t.Fatal("failed persistence discarded pending source history")
	}
}

func TestSourceExitClassificationsKeepExternalReaderFailureSeparateFromCancellation(t *testing.T) {
	for _, tc := range []struct {
		cause error
		want  string
	}{{nil, "stopped"}, {context.Canceled, "cancelled"}, {sourceReadFailure(io.EOF), "eof"}, {sourceReadFailure(io.ErrUnexpectedEOF), "truncated"}, {&sourceReaderFailure{operation: "source-reader-authority", cause: ErrAuthority}, "authority"}} {
		if got := sourceReaderOutcome(tc.cause); got != tc.want {
			t.Fatal("reader cause lost", got, tc.want)
		}
	}
	if sourceCommandOutcome(peer.ErrBoundaryInspectionUnavailable) != "inspection" || sourceCommandOutcome(errors.Join(peer.ErrBoundaryInspectionUnavailable, ErrRecovery)) != "cleanup" || sourceCommandOutcome(ErrAuthority) != "authority" {
		t.Fatal("command outcome collapsed independent cleanup/authority failure")
	}
}

func TestSourceExitPublicationCannotWaitForSlowRuntimePersistence(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	r := newLoginRuntimeRecorder(func(logindiag.Runtime) error {
		close(entered)
		<-release
		return nil
	})
	recorded := make(chan struct{})
	go func() { r.record("service", "READY", nil); close(recorded) }()
	<-entered
	defer func() { close(release); <-recorded }()
	exit := cleanSourceExit()
	published := make(chan struct{})
	go func() { r.recordSourceExit(Report{Source: &exit}); close(published) }()
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("source endpoint blocked behind diagnostic persistence")
	}
	if r.source.Load() == nil || r.source.Load().Total != 1 {
		t.Fatal("source publication lost while old snapshot persisted")
	}
}
