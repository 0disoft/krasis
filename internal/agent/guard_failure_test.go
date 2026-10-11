package agent

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/crashguard"
	productlog "github.com/0disoft/krasis/internal/failurecode"
	"testing"
)

func TestGuardFailuresAreNotGenericCancellation(t *testing.T) {
	for _, test := range []struct {
		cause error
		want  string
	}{
		{crashguard.ErrUnexpectedExit, "lifecycle_exited"},
		{&crashguard.Failure{Operation: "prepare", Cause: crashguard.ErrIPCTimeout}, "prepare_timeout"},
		{&crashguard.Failure{Operation: "close", Cause: crashguard.ErrRecoveryFailed}, "close_recovery_failed"},
		{&crashguard.Failure{Operation: "close", Cause: crashguard.ErrExitTimeout}, "close_exit_timeout"},
	} {
		err := errors.Join(context.Canceled, test.cause, ErrRecoveryUnverified)
		d := DescribeSessionFailure(err)
		if got := productlog.FaultCode(d.Component, d.Phase, d.Code); got != "fault_guard_"+test.want {
			t.Fatalf("lost guard cause: %s", got)
		}
	}
}
