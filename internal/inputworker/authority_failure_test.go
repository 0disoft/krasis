package inputworker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"syscall"
	"testing"

	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/logindiag"
)

func TestSourceAuthorityDetailsKeepSentinelContractAndRetirementOutcome(t *testing.T) {
	for _, operation := range []string{"authority-process-query", "authority-process-exited", "authority-console-session", "authority-console-token", "authority-logon-token", "authority-logon-changed", "authority-service-pipe", "authority-service-status"} {
		for _, native := range []syscall.Errno{0, 5, 170} {
			cause := authorityCheckFailed(operation, native)
			a := &Authority{checkInput: func() error { return cause }, checkPresent: func() error { return cause }, checkRecovery: func() error { return cause }, closeHandles: func() error { return nil }}
			if a.check(false) != ErrAuthority || a.check(true) != ErrAuthority || a.checkHeartbeat() != ErrAuthority {
				t.Fatal("ordinary authority contract changed", operation)
			}
			for _, result := range []error{a.checkSource(), a.checkSourceHeartbeat()} {
				if result == nil || result.Error() != ErrAuthority.Error() || !errors.Is(result, ErrAuthority) || native != 0 && !errors.Is(result, native) {
					t.Fatal("source error lost denial or leaked detail", result)
				}
				loop := &sourceReaderFailure{operation: "source-reader-authority", cause: result}
				final := sourceCaptureFinal(loop, inputdesktop.ErrInactive, context.Canceled, false)
				if final == nil || errors.Is(final, ErrRecovery) || sourceRetirementStatus(final) != 0 {
					t.Fatal("diagnostics changed joined retirement", operation, final)
				}
				var wire bytes.Buffer
				exit := cleanSourceExit()
				exit.Report = "failed"
				if err := writeReport(&wire, Report{Source: &exit}, final); err != nil {
					t.Fatal(err)
				}
				report, received := readReport(&wire)
				if received == nil || !errors.Is(received, ErrAuthority) || serviceRecoveryFailure(received, nil) != nil {
					t.Fatal("source denial became success or service failure", received)
				}
				r := newLoginRuntimeRecorder(func(logindiag.Runtime) error { return nil })
				r.record("service", "READY", nil)
				r.recordSourceExit(report, received)
				r.record("console", "USER_SESSION", nil)
				failure := r.value.Source.Exits[0].Failure
				if !r.value.Valid() || failure == nil || failure.Operation != operation || failure.Win32Error != uint32(native) || failure.Win32Known != (native != 0) {
					t.Fatal("real wire/recorder lost failed check", operation, failure)
				}
				// A separate failed native owner must still fence the service.
				uncertain := sourceCaptureFinal(loop, inputdesktop.ErrCleanup, context.Canceled, false)
				if !errors.Is(uncertain, ErrRecovery) || sourceRetirementStatus(uncertain) != 1 {
					t.Fatal("authority details erased unknown cleanup", uncertain)
				}
			}
			_ = a.Close()
			if a.checkSource() != ErrAuthority || a.checkSourceHeartbeat() != ErrAuthority {
				t.Fatal("closed proof accepted")
			}
		}
	}
}

func TestSourceAuthorityProjectionStripsUnknownDetailsAndBindsNativeToOperation(t *testing.T) {
	private := errors.New("SECRET/PIN/path")
	for _, cause := range []error{private, connectionFailed("PRIVATE", syscall.Errno(5)), authorityCheckFailed("PRIVATE", private)} {
		if sourceAuthorityResult(cause) != ErrAuthority {
			t.Fatal("unknown authority cause retained")
		}
	}
	for _, tc := range []struct{ stage, operation string }{{"service_pid", "authority-service-pipe"}, {"service_status", "authority-service-status"}} {
		err := sourceAuthorityResult(errors.Join(syscall.Errno(109), connectionFailed(tc.stage, errors.Join(private, syscall.Errno(5)))))
		// An unrelated error earlier in a joined tree cannot supply the native
		// number for the selected authority operation.
		op, native := reportFailureDetails(errors.Join(syscall.Errno(109), err))
		if reportOperations[op] != tc.operation || native != 5 || err.Error() != ErrAuthority.Error() {
			t.Fatal("operation/native pairing or public error changed", op, native, err)
		}
		data, _ := json.Marshal(err)
		if bytes.Contains(data, []byte("SECRET")) || bytes.Contains(data, []byte("PIN")) || bytes.Contains(data, []byte("path")) || errors.Is(err, private) {
			t.Fatal("private source error retained", string(data))
		}
	}
	for _, a := range []*Authority{nil, {}, {checkInput: func() error { t.Fatal("incomplete proof called"); return nil }}} {
		if a.checkSource() != ErrAuthority || a.checkSourceHeartbeat() != ErrAuthority {
			t.Fatal("zero source proof accepted")
		}
	}
}
