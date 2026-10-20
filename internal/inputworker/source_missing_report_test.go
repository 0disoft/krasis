package inputworker

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/logindiag"
)

func TestMissingSourceReportKeepsStageWithoutInventingNativeState(t *testing.T) {
	for _, stage := range []string{"admission", "request", "validation", "prepare", "authentication", "transfer", "ready", "publish"} {
		r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
		r.record("service", "READY", nil)
		cause := errors.Join(ErrRecovery, syscall.Errno(5))
		result := sourceStartupFailure(stage, Report{}, cause)
		if !errors.Is(result, cause) || !errors.Is(result, ErrRecovery) {
			t.Fatal("failure ownership changed", result)
		}
		r.recordSourceExit(Report{}, result)
		r.record("console", "USER_SESSION", nil)
		if !r.value.Valid() || r.value.Source == nil || r.value.Source.Total != 1 {
			t.Fatal("missing report was discarded", r.value)
		}
		exit := r.value.Source.Exits[0]
		if exit.Report != "missing" || exit.Command != "unknown" || exit.Reader != "unknown" || exit.Capture != "unknown" || exit.Sender != "unknown" || exit.Interrupt != "unknown" || exit.Retirement != "unknown" || exit.Failure == nil || exit.Failure.Operation != "source-startup-"+stage || exit.Failure.Win32Error != 5 || !exit.Failure.Win32Known {
			t.Fatal("startup failure was lost or invented native state", exit)
		}
		if err := writeReport(io.Discard, Report{Source: &exit}, result); err != ErrProtocol {
			t.Fatal("service history entered worker proof", err)
		}
		// Even stripping service-only metadata cannot turn unknown state into
		// an authenticated worker receipt, under any status.
		exit.Time, exit.Failure = "", nil
		for status := byte(0); status <= 2; status++ {
			if _, err := encodeSourceExit(exit, status); err != ErrProtocol {
				t.Fatal("missing report became wire proof", status, err)
			}
		}
	}
}

func TestSourceStartupStageCannotReplaceWorkerFailureOrLeakRawDetails(t *testing.T) {
	private := errors.New("SECRET/path/input")
	exit := cleanSourceExit()
	if sourceStartupFailure("prepare", Report{Source: &exit}, private) != private || sourceStartupFailure("prepare", Report{}, nil) != nil || sourceStartupFailure("SECRET/path", Report{}, private) != private {
		t.Fatal("stage replaced existing evidence or accepted arbitrary operation")
	}
	r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
	r.recordSourceExit(Report{}, nil)
	if r.source.Load() != nil {
		t.Fatal("successful empty result became missing-report failure")
	}
	r.recordSourceExit(Report{}, sourceStartupFailure("prepare", Report{}, private))
	data, _ := json.Marshal(r.source.Load())
	if bytes.Contains(data, []byte("SECRET")) {
		t.Fatal("private error leaked")
	}
	missing := r.source.Load().Exits[0]
	missing.Capture = "joined"
	if missing.ValidDetails() {
		t.Fatal("missing report claimed native join")
	}
}
