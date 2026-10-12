package inputworker

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/logindiag"
	"syscall"
	"testing"
)

func TestLoginDecisionRecordingDeduplicatesAndKeepsFailureAfterLogin(t *testing.T) {
	var saved []logindiag.Runtime
	r := newLoginRuntimeRecorder(func(v logindiag.Runtime) error { saved = append(saved, v); return nil })
	r.record("service", "STARTING", nil)
	r.record("service", "READY", nil)
	r.record("console", "BEFORE_LOGIN", nil)
	for range 10 {
		r.record("console", "BEFORE_LOGIN", nil)
	}
	if len(saved) != 3 {
		t.Fatal("recorded every policy poll", len(saved))
	}
	r.record("start", "FAILED", syscall.Errno(5))
	r.record("console", "USER_SESSION", nil)
	last := saved[len(saved)-1]
	if !last.Valid() || last.LastFailure == nil || last.LastFailure.Stage != "start" || last.LastFailure.Win32Error != 5 || !last.LastFailure.Win32Known {
		t.Fatal("login erased startup failure", last)
	}
	if last.StartedAt != saved[0].StartedAt {
		t.Fatal("boot time changed")
	}
}

func TestFailedDecisionPersistenceNeverMarksSaved(t *testing.T) {
	calls := 0
	r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { calls++; return errors.New("disk unavailable") })
	r.record("service", "STARTING", nil)
	r.record("service", "STARTING", nil)
	if calls != 2 || r.value.Time != "" {
		t.Fatal("unwritten receipt suppressed future save")
	}
}

func TestServiceEndpointFailureKeepsClassificationAndNativeCause(t *testing.T) {
	r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
	r.record("service", "FAILED", errors.Join(serviceEndpointFailure(errors.Join(ErrRecovery, syscall.Errno(5)), "capture"), context.Canceled))
	if r.value.LastFailure == nil || r.value.LastFailure.Reason != "capture" || !r.value.LastFailure.Win32Known || r.value.LastFailure.Win32Error != 5 {
		t.Fatal("service lost exact endpoint/native failure", r.value)
	}
}
