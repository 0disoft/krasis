package inputworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"testing/iotest"

	"github.com/0disoft/krasis/internal/logindiag"
)

func TestMissingWorkerReportPreservesEOFTruncationAndInvalidKinds(t *testing.T) {
	var encoded bytes.Buffer
	if err := writeReport(&encoded, Report{}, nil); err != nil {
		t.Fatal(err)
	}
	invalid := append([]byte(nil), encoded.Bytes()...)
	invalid[0] = 0
	for _, tc := range []struct {
		name      string
		data      []byte
		operation string
	}{
		{"eof", nil, "worker-report-eof"},
		{"truncated", encoded.Bytes()[:3], "worker-report-truncated"},
		{"invalid", invalid, "worker-report-invalid"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			report, err := readReport(bytes.NewReader(tc.data))
			_, err = receipt(report, err)
			calls := 0
			err = observeMissingWorkerReport(err, func() logindiag.WorkerExit {
				calls++
				return logindiag.WorkerExit{State: "EXITED", Code: 0xc0000005, Known: true}
			})
			r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
			r.record("service", "FAILED", serviceEndpointFailure(err, "input"))
			f := r.value.LastFailure
			if calls != 1 || !errors.Is(err, ErrRecovery) || f == nil || f.Operation != tc.operation || f.WorkerExit == nil || f.WorkerExit.Code != 0xc0000005 || f.Win32Known || !r.value.Valid() {
				t.Fatal("missing report class or exit metadata erased", err, f)
			}
		})
	}
}

func TestWorkerReportDeadlineRetainsTimeoutAndRecoveryFence(t *testing.T) {
	report, err := readReport(iotest.ErrReader(os.ErrDeadlineExceeded))
	_, err = receipt(report, err)
	err = observeMissingWorkerReport(err, func() logindiag.WorkerExit { return logindiag.WorkerExit{State: "RUNNING"} })
	var operation interface{ LoginRuntimeOperation() string }
	if !errors.Is(err, ErrRecovery) || !errors.Is(err, os.ErrDeadlineExceeded) || !errors.As(err, &operation) || operation.LoginRuntimeOperation() != "worker-report-timeout" {
		t.Fatal("read timeout mislabeled or unfenced", err)
	}
}

func TestMissingWorkerReportKeepsExactProcessStatusSeparateFromWin32(t *testing.T) {
	for _, observed := range []logindiag.WorkerExit{
		{State: "RUNNING"}, {State: "UNAVAILABLE"}, {State: "EXITED"},
		{State: "EXITED", Known: true}, {State: "EXITED", Code: 0xc0000005, Known: true},
	} {
		_, missing := receipt(Report{}, &workerReportFailure{operation: "worker-report-read", cause: errors.Join(ErrProtocol, io.EOF, errors.New("SECRET"))})
		calls := 0
		result := observeMissingWorkerReport(missing, func() logindiag.WorkerExit { calls++; return observed })
		if calls != 1 || !errors.Is(result, missing) || !errors.Is(result, ErrRecovery) {
			t.Fatal("process query erased the missing-report fence", result)
		}
		r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
		r.record("service", "FAILED", serviceEndpointFailure(result, "input"))
		f := r.value.LastFailure
		if f == nil || f.Operation != "worker-report-read" || f.Win32Known || f.WorkerExit == nil || *f.WorkerExit != observed || !r.value.Valid() {
			t.Fatal("process exit became a Win32 error or disappeared", f)
		}
		encoded, _ := json.Marshal(r.value)
		if strings.Contains(string(encoded), "SECRET") {
			t.Fatal("private failure content entered diagnostics")
		}
	}
}

func TestWorkerReportStatusQueryCannotConvertFailureToRecoverySuccess(t *testing.T) {
	for _, result := range []error{nil, ErrAuthority, ErrRecovery, &workerReportFailure{operation: "cursor-restore", cause: ErrRecovery}} {
		if got := observeMissingWorkerReport(result, func() logindiag.WorkerExit { t.Fatal("unrelated error inspected"); return logindiag.WorkerExit{} }); got != result {
			t.Fatal("unrelated result changed")
		}
	}
	r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
	_, missing := receipt(Report{}, ErrProtocol)
	for _, state := range []logindiag.WorkerExit{{State: "RUNNING"}, {State: "EXITED", Known: true}, {State: "EXITED", Code: 2, Known: true}} {
		err := observeMissingWorkerReport(missing, func() logindiag.WorkerExit { return state })
		r.record("service", "FAILED", serviceEndpointFailure(err, "input"))
		if *r.value.LastFailure.WorkerExit != state || !errors.Is(err, ErrRecovery) {
			t.Fatal("changed exit status was deduplicated or zero exit admitted new leases")
		}
	}
}
