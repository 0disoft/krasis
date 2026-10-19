package inputworker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestCaptureThreadFailureDetailsSurviveReportAndSanitizedRuntimeReceipt(t *testing.T) {
	operations := append(inputdesktop.CaptureThreadFailureOperations(), "thread-join-low-level-hook", "thread-join-raw-mouse", "thread-join-wheel-capture", "thread-join-multiple")
	for _, operation := range operations {
		var wire bytes.Buffer
		failure := &workerReportFailure{operation: operation, cause: errors.Join(ErrRecovery, syscall.Errno(1460))}
		if err := writeReport(&wire, Report{}, failure); err != nil {
			t.Fatal(err)
		}
		_, received := readReport(&wire)
		recorder := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
		recorder.record("service", "FAILED", serviceEndpointFailure(received, "capture"))
		actual := recorder.value.LastFailure
		if !errors.Is(received, ErrRecovery) || !errors.Is(received, syscall.Errno(1460)) || actual == nil || actual.Operation != operation || !actual.Win32Known || actual.Win32Error != 1460 || !recorder.value.Valid() {
			t.Fatal("capture retirement evidence lost", operation, actual)
		}
	}
	if len(reportOperations) > 256 {
		t.Fatal("operation vocabulary exceeds its wire byte")
	}
}

func TestWorkerFailureReceiptPreservesOperationNativeCauseAndRecoveryFence(t *testing.T) {
	// Construct the exact fixed operation code the writer obtains from a source
	// stage. The arbitrary private error is never serialized.
	for _, operation := range []struct {
		code   byte
		name   string
		native uint32
	}{{15, "cursor-restore", 5}, {17, "raw-size", 170}, {18, "raw-packet", 170}, {19, "raw-desktop", 170}, {21, "desktop-restore", 170}, {25, "raw-window-destroy", 1400}} {
		for _, status := range []byte{1, 2} {
			var buffer bytes.Buffer
			if err := writeReport(&buffer, Report{}, ErrAuthority); err != nil {
				t.Fatal(err)
			}
			data := buffer.Bytes()
			data[5], data[6] = status, operation.code
			binary.BigEndian.PutUint32(data[28:32], operation.native)
			_, err := readReport(bytes.NewReader(data))
			if !errors.Is(err, syscall.Errno(operation.native)) || errors.Is(err, ErrRecovery) != (status == 2) {
				t.Fatal("native cause or recovery status lost", err)
			}
			r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
			r.record("service", "FAILED", serviceEndpointFailure(err, "capture"))
			f := r.value.LastFailure
			if f == nil || f.Reason != "capture" || f.Operation != operation.name || f.Win32Error != operation.native || !f.Win32Known || !r.value.Valid() {
				t.Fatal("child failure lost across service receipt", f)
			}
			r.record("service", "FAILED", &workerReportFailure{operation: "cursor-release", cause: err})
			if r.value.LastFailure.Operation != "cursor-release" {
				t.Fatal("operation-only change was deduplicated")
			}
		}
	}
}

func TestWorkerFailureReceiptRejectsUnknownDiagnosticsAndKeepsLegacyReceipts(t *testing.T) {
	var buffer bytes.Buffer
	_ = writeReport(&buffer, Report{}, nil)
	data := append([]byte(nil), buffer.Bytes()...)
	data[4] = 1
	if _, err := readReport(bytes.NewReader(data)); err != nil {
		t.Fatal("legacy clean report rejected", err)
	}
	for _, corrupt := range []func([]byte){
		func(b []byte) { b[6] = 255 },
		func(b []byte) { b[7] = 1 },
		func(b []byte) { b[4], b[6] = 1, 1 },
		func(b []byte) { b[28] = 1 }, // A success must not contain a failure.
	} {
		bad := append([]byte(nil), buffer.Bytes()...)
		corrupt(bad)
		if _, err := readReport(bytes.NewReader(bad)); err != ErrProtocol {
			t.Fatal("malformed failure metadata accepted", err)
		}
	}
	for code := range reportOperations {
		if !validReportOperation(byte(code)) {
			t.Fatal("operation vocabulary drifted", code)
		}
	}
	if logindiag.ValidRuntimeOperation("PRIVATE/window/path") {
		t.Fatal("private operation accepted")
	}
}

func TestReportDetailsKeepOperationAndNativeCauseFromSameBranch(t *testing.T) {
	for _, operation := range []string{"source-reader-read", "source-retirement-write", "worker-report-timeout"} {
		cause := errors.Join(syscall.Errno(5), &workerReportFailure{operation: operation, cause: syscall.Errno(170)})
		op, native := reportFailureDetails(cause)
		// Service-only report operations are deliberately not on the child wire.
		if operation != "worker-report-timeout" && reportOperations[op] != operation || native != 170 {
			t.Fatal("selected report operation used sibling native cause", operation, op, native)
		}
	}
}

func TestConcreteCleanupReportCannotBeReplacedByAuthorityDetail(t *testing.T) {
	for _, operation := range []string{"hook-release", "raw-release", "cursor-restore", "desktop-close"} {
		cleanup := &workerReportFailure{operation: operation, cause: errors.Join(ErrRecovery, syscall.Errno(170))}
		combined := errors.Join(cleanup, authorityCheckFailed("authority-console-token", syscall.Errno(5)))
		var wire bytes.Buffer
		if err := writeReport(&wire, Report{}, combined); err != nil {
			t.Fatal(err)
		}
		_, received := readReport(&wire)
		op, native := reportFailureDetails(received)
		if reportOperations[op] != operation || native != 170 || !errors.Is(received, ErrRecovery) || serviceRecoveryFailure(received, nil) == nil {
			t.Fatal("authority diagnostic obscured concrete cleanup failure", operation, op, native, received)
		}
	}
}
