package inputworker

import (
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/application"
)

func TestIsolatedJournalPreservesNativeOutcomeWithoutParentQueries(t *testing.T) {
	for _, certainty := range []application.InjectionCertainty{application.InjectionNotApplied, application.InjectionPartiallyApplied, application.InjectionOutcomeUnknown} {
		input := &isolatedInput{Client: &Client{}}
		ownerCalls := 0
		owner := func(context.Context, func() error) error { ownerCalls++; return ErrAuthority }
		timing := &workerTiming{measured: 7}
		failure := &application.InjectionError{Certainty: certainty, Requested: 2, Inserted: 1, Cause: ErrAuthority}
		err := journalExecution(input, owner, timing)(context.Background(), func() error { return failure })
		if err != failure || ownerCalls != 0 || timing.measured != 4 || timing.buckets[0] != 0 || timing.buckets[1] != 0 || timing.desktop != 0 {
			t.Fatal("journal changed child outcome or invented native observations", err, ownerCalls, timing)
		}
	}
}

func TestJournalCannotBypassNativeOwnerForDirectPorts(t *testing.T) {
	ownerCalls, inputCalls := 0, 0
	owner := func(context.Context, func() error) error { ownerCalls++; return ErrAuthority }
	err := journalExecution(&recordingInput{}, owner, nil)(context.Background(), func() error { inputCalls++; return nil })
	if err != ErrAuthority || ownerCalls != 1 || inputCalls != 0 {
		t.Fatal(err, ownerCalls, inputCalls)
	}
}

func TestIsolatedJournalRejectsCancelledRetiredOrMissingTransportBeforeDispatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	for _, test := range []struct {
		ctx   context.Context
		input *isolatedInput
	}{
		{ctx, &isolatedInput{Client: &Client{}}},
		{nil, &isolatedInput{Client: &Client{}}},
		{context.Background(), &isolatedInput{Client: &Client{}, recovering: true}},
		{context.Background(), &isolatedInput{}},
		{context.Background(), nil},
	} {
		called := false
		err := journalExecution(test.input, testExecute, nil)(test.ctx, func() error { called = true; return nil })
		certainty, known := application.InjectionCertaintyOf(err)
		if err == nil || called || !known || certainty != application.InjectionNotApplied {
			t.Fatal(err, called, certainty)
		}
	}
}

func TestLoginJournalRetainsParentRecoveryAuthorityAfterChildRetirement(t *testing.T) {
	for _, denyRecovery := range []bool{false, true} {
		recovery := &recordingInput{}
		childCalls, ownerCalls := 0, 0
		native := &isolatedInput{Client: &Client{localExchange: func(context.Context, request, func(response)) error { childCalls++; return nil }}, recovery: recovery, retire: func() (bool, error) { return true, nil }}
		owner := func(ctx context.Context, work func() error) error {
			ownerCalls++
			if !native.recovering {
				return errors.New("parent queried live input")
			}
			if denyRecovery {
				return ErrAuthority
			}
			return loginTestDo(ctx, work)
		}
		report, err := runLoginInput(context.Background(), native, fakeLoginBind(owner), func(ctx context.Context, port application.InputPort) error {
			if err := port.Press(ctx, loginTestKey); err != nil {
				return err
			}
			return port.Move(ctx, 1, 0)
		})
		if childCalls != 2 || ownerCalls != 1 {
			t.Fatal("wrong owner executed input or recovery", childCalls, ownerCalls)
		}
		if denyRecovery {
			if !errors.Is(err, ErrRecovery) || report.Recovery.Remaining != 1 || recovery.released != 0 {
				t.Fatal(report, err)
			}
		} else if err != nil || report.Recovery.Released != 1 || report.Recovery.Remaining != 0 || recovery.released != 1 {
			t.Fatal(report, err)
		}
	}
}
