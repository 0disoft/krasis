package agent

import (
	"errors"
	"github.com/0disoft/krasis/internal/application"
	productlog "github.com/0disoft/krasis/internal/failurecode"
	"github.com/0disoft/krasis/internal/peer"
	"testing"
)

func TestInjectionFailureRemainsDistinctFromPeerDispatchAndCleanup(t *testing.T) {
	for _, fixture := range []struct {
		certainty application.InjectionCertainty
		code      string
	}{
		{application.InjectionNotApplied, "not_applied"},
		{application.InjectionPartiallyApplied, "partially_applied"},
		{application.InjectionOutcomeUnknown, "outcome_unknown"},
	} {
		err := errors.Join(peer.ErrPeerInputDispatch, &application.InjectionError{Certainty: fixture.certainty}, ErrRecoveryUnverified)
		d := DescribeSessionFailure(err)
		if got := productlog.FaultCode(d.Component, d.Phase, d.Code); got != "fault_injection_"+fixture.code {
			t.Fatalf("lost injection classification: %s", got)
		}
		if !errors.Is(err, ErrRecoveryUnverified) {
			t.Fatal("cleanup failure lost")
		}
	}
}
