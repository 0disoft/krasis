package inputworker

import (
	"bytes"
	"encoding/binary"
	"errors"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/logindiag"
)

func TestWorkerFailureReceiptPreservesOperationNativeCauseAndRecoveryFence(t *testing.T) {
	// Construct the exact fixed operation code the writer obtains from a source
	// stage. The arbitrary private error is never serialized.
	for _, status := range []byte{1, 2} {
		var buffer bytes.Buffer
		if err := writeReport(&buffer, Report{}, ErrAuthority); err != nil {
			t.Fatal(err)
		}
		data := buffer.Bytes()
		data[5], data[6] = status, 15 // cursor-restore
		binary.BigEndian.PutUint32(data[28:32], 5)
		_, err := readReport(bytes.NewReader(data))
		if !errors.Is(err, syscall.Errno(5)) || errors.Is(err, ErrRecovery) != (status == 2) {
			t.Fatal("native cause or recovery status lost", err)
		}
		r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
		r.record("service", "FAILED", serviceEndpointFailure(err, "capture"))
		f := r.value.LastFailure
		if f == nil || f.Reason != "capture" || f.Operation != "cursor-restore" || f.Win32Error != 5 || !f.Win32Known || !r.value.Valid() {
			t.Fatal("child failure lost across service receipt", f)
		}
		r.record("service", "FAILED", &workerReportFailure{operation: "cursor-release", cause: err})
		if r.value.LastFailure.Operation != "cursor-release" {
			t.Fatal("operation-only change was deduplicated")
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
