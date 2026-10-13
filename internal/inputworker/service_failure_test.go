package inputworker

import (
	"bytes"
	"errors"
	"io"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestServiceRecoveryRetainsWorkerAndConcurrentCloseFailure(t *testing.T) {
	worker := &workerReportFailure{operation: "cursor-restore", cause: errors.Join(ErrRecovery, syscall.Errno(5))}
	closeErr := errors.New("private close failure")
	for _, endpoint := range []string{"capture", "input"} {
		for _, cleanup := range []error{nil, closeErr} {
			_, received := receipt(Report{Recovery: application.ReleaseReport{Remaining: 1}}, worker)
			failure := serviceRecoveryFailure(received, cleanup)
			if !errors.Is(failure, ErrRecovery) || !errors.Is(failure, syscall.Errno(5)) || cleanup != nil && !errors.Is(failure, cleanup) {
				t.Fatal("service fence erased original worker or close failure", failure)
			}
			r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
			r.record("service", "FAILED", serviceEndpointFailure(failure, endpoint))
			f := r.value.LastFailure
			if f == nil || f.Reason != endpoint || f.Operation != "cursor-restore" || f.Win32Error != 5 || !f.Win32Known || !r.value.Valid() {
				t.Fatal("original worker failure missing from service receipt", f)
			}
		}
	}
	if serviceRecoveryFailure(nil, nil) != nil || serviceRecoveryFailure(ErrAuthority, nil) != nil {
		t.Fatal("ordinary rejection unexpectedly fenced service")
	}
	if err := serviceRecoveryFailure(nil, closeErr); !errors.Is(err, ErrRecovery) || !errors.Is(err, closeErr) {
		t.Fatal("close uncertainty did not fence service", err)
	}
}

func TestMissingWorkerReportIsDistinctFromUnknownNativeFailure(t *testing.T) {
	for _, size := range []int{0, 1, reportSize - 1} {
		report, readErr := readReport(bytes.NewReader(make([]byte, size)))
		_, failure := receipt(report, readErr)
		if !errors.Is(failure, ErrProtocol) || !errors.Is(failure, ErrRecovery) || !errors.Is(failure, io.EOF) && !errors.Is(failure, io.ErrUnexpectedEOF) {
			t.Fatal("missing report lost read failure or recovery fence", failure)
		}
		r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
		r.record("service", "FAILED", serviceEndpointFailure(serviceRecoveryFailure(failure, nil), "capture"))
		f := r.value.LastFailure
		if f == nil || f.Operation != "worker-report-read" || f.Win32Known || f.Win32Error != 0 || !r.value.Valid() {
			t.Fatal("missing report became unexplained native failure", f)
		}
	}
}
