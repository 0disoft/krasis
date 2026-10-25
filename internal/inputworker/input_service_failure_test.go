package inputworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"syscall"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/logindiag"
)

type inputRetirementFixture struct {
	io.Reader
	written                         bytes.Buffer
	deadlineErr, writeErr           error
	shortWrite                      bool
	deadlineCalls, failDeadlineCall int
}

func (f *inputRetirementFixture) SetDeadline(time.Time) error {
	f.deadlineCalls++
	if f.failDeadlineCall == 0 || f.deadlineCalls == f.failDeadlineCall {
		return f.deadlineErr
	}
	return nil
}
func (f *inputRetirementFixture) Write(data []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	if f.shortWrite {
		return len(data) - 1, nil
	}
	return f.written.Write(data)
}

func TestInputRetirementPreservesStageCauseAndRecoveryFence(t *testing.T) {
	retired := [8]byte{'K', 'R', 'U', '1', 1, 0, 0, 0}
	for _, tc := range []struct {
		name, stage                 string
		response                    []byte
		deadlineErr, writeErr, want error
		shortWrite                  bool
	}{
		{name: "deadline", stage: "deadline", deadlineErr: os.ErrDeadlineExceeded, want: os.ErrDeadlineExceeded},
		{name: "foreign authentication", stage: "authentication", response: retired[:], want: ErrProtocol},
		{name: "truncated authentication", stage: "authentication", response: workerAuthenticatedFrame[:3], want: io.ErrUnexpectedEOF},
		{name: "stop", stage: "stop", response: workerAuthenticatedFrame[:], writeErr: syscall.Errno(109), want: syscall.Errno(109)},
		{name: "short stop", stage: "stop", response: workerAuthenticatedFrame[:], shortWrite: true, want: io.ErrShortWrite},
		{name: "missing receipt", stage: "receipt", response: workerAuthenticatedFrame[:], want: io.EOF},
		{name: "truncated receipt", stage: "receipt", response: append(append([]byte{}, workerAuthenticatedFrame[:]...), retired[:3]...), want: io.ErrUnexpectedEOF},
		{name: "foreign receipt", stage: "receipt", response: append(append([]byte{}, workerAuthenticatedFrame[:]...), workerAuthenticatedFrame[:]...), want: ErrProtocol},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := &inputRetirementFixture{Reader: bytes.NewReader(tc.response), deadlineErr: tc.deadlineErr, writeErr: tc.writeErr, shortWrite: tc.shortWrite}
			result := retireInputPreparation(f, workerAuthenticatedFrame, retired)
			if !errors.Is(result, ErrRecovery) || !errors.Is(result, tc.want) {
				t.Fatal("retirement lost recovery or original cause", result)
			}
			r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
			r.record("service", "FAILED", serviceEndpointFailure(result, "input"))
			failure := r.value.LastFailure
			if failure == nil || failure.Operation != "input-retire-"+tc.stage || failure.Reason != "input" || !r.value.Valid() {
				t.Fatal("retirement stage did not reach runtime", failure)
			}
			if tc.stage == "deadline" || tc.stage == "authentication" {
				if f.written.Len() != 0 {
					t.Fatal("stop sent before authentication")
				}
			}
			if tc.stage == "receipt" && (f.written.Len() != bootstrapSize || !zeroBytes(f.written.Bytes())) {
				t.Fatal("retirement changed reserved stop bootstrap")
			}
		})
	}
	f := &inputRetirementFixture{Reader: io.MultiReader(bytes.NewReader(workerAuthenticatedFrame[:]), bytes.NewReader(retired[:]))}
	if err := retireInputPreparation(f, workerAuthenticatedFrame, retired); err != nil || f.written.Len() != bootstrapSize || !zeroBytes(f.written.Bytes()) {
		t.Fatal("exact receipt rejected or lease data sent", err)
	}
}

func TestInputRetirementReceiptDeadlineFailureKeepsRecoveryFence(t *testing.T) {
	f := &inputRetirementFixture{
		Reader:      bytes.NewReader(append(append([]byte{}, workerAuthenticatedFrame[:]...), unboundRetiredFrame[:]...)),
		deadlineErr: syscall.Errno(6), failDeadlineCall: 2,
	}
	err := retireInputPreparation(f, workerAuthenticatedFrame, unboundRetiredFrame)
	if !errors.Is(err, ErrRecovery) || !errors.Is(err, syscall.Errno(6)) || f.deadlineCalls != 2 || f.written.Len() != bootstrapSize || !zeroBytes(f.written.Bytes()) {
		t.Fatal("receipt deadline rejection lost the fence or stop", err)
	}
	remaining, _ := io.ReadAll(f.Reader)
	if !bytes.Equal(remaining, unboundRetiredFrame[:]) {
		t.Fatal("receipt consumed after deadline setup failed")
	}
}

func TestInputServicePhaseKeepsSpecificFailureAndNativeCode(t *testing.T) {
	specific := &workerReportFailure{operation: "worker-report-timeout", cause: errors.Join(ErrRecovery, os.ErrDeadlineExceeded)}
	if inputServiceAt("input-prepare-connect", specific) != specific || inputServiceAt("SECRET/path", ErrAuthority) != ErrAuthority || inputServiceAt("input-prepare-launch", nil) != nil {
		t.Fatal("phase replaced specific failure or accepted private operation")
	}
	native := inputServiceAt("input-prepare-launch", errors.Join(ErrAuthority, syscall.Errno(5)))
	if !errors.Is(native, ErrAuthority) || !errors.Is(native, syscall.Errno(5)) {
		t.Fatal("phase erased cause")
	}
	// Service-local phases must not allocate child wire operation codes.
	if code, number := reportFailureDetails(native); code != 0 || number != 5 {
		t.Fatal("service detail changed wire operation or lost native cause", code, number)
	}
	if inputServiceAt("input-startup-prepare", native) != native {
		t.Fatal("generic startup hid preparation stage")
	}
}

func TestInputPreparationExitCannotAcknowledgeMissingRetirement(t *testing.T) {
	for _, exit := range []logindiag.WorkerExit{
		{State: "RUNNING"}, {State: "UNAVAILABLE"}, {State: "EXITED"},
		{State: "EXITED", Known: true}, {State: "EXITED", Code: 0xc0000005, Known: true},
	} {
		cause := inputServiceAt("input-retire-receipt", errors.Join(ErrRecovery, io.EOF, errors.New("SECRET/path")))
		result := observeInputPreparationExit(cause, func() logindiag.WorkerExit { return exit })
		if !errors.Is(result, cause) || !errors.Is(result, ErrRecovery) || !errors.Is(result, io.EOF) {
			t.Fatal("exit cleared retirement fence")
		}
		r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
		r.record("service", "FAILED", serviceEndpointFailure(result, "input"))
		f := r.value.LastFailure
		if f == nil || f.Operation != "input-retire-receipt" || f.WorkerExit == nil || *f.WorkerExit != exit || f.Win32Known || !r.value.Valid() {
			t.Fatal("exit replaced native cause or stage", f)
		}
		data, err := json.Marshal(r.value)
		if err != nil || bytes.Contains(data, []byte("SECRET")) {
			t.Fatal("private cause entered diagnostic", err)
		}
	}
	if observeInputPreparationExit(nil, func() logindiag.WorkerExit { t.Fatal("success inspected"); return logindiag.WorkerExit{} }) != nil {
		t.Fatal("success changed")
	}
	cause := inputServiceAt("input-retire-receipt", ErrRecovery)
	if observeInputPreparationExit(cause, nil) != cause || observeInputPreparationExit(cause, func() logindiag.WorkerExit { return logindiag.WorkerExit{} }) != cause {
		t.Fatal("unknown query cleared failure")
	}
}
