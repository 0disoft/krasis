package inputworker

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/inputdesktop"
	"github.com/0disoft/krasis/internal/wininput"
)

type activationTestSink struct{}

func (*activationTestSink) SendAction(context.Context, control.InputAction, func() bool) error {
	return nil
}
func (*activationTestSink) Interrupt(error) error             { return nil }
func (*activationTestSink) Fail(context.Context, error) error { return nil }

func TestSessionActivationRejectionKeepsJoinedFailureLocalToWorker(t *testing.T) {
	session, err := wininput.NewSourceSession(1, &activationTestSink{})
	if err != nil {
		t.Fatal(err)
	}
	run, cancel := context.WithCancel(context.Background())
	defer cancel()
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- session.RunWithReady(run, func() { close(ready) }) }()
	<-ready
	if err := activateSourceSession(session); err != nil {
		t.Fatal(err)
	}
	rejected := activateSourceSession(session)
	if rejected == nil || !session.Active() {
		t.Fatal("fixture did not reject an already active session without changing it", rejected)
	}
	cancel()
	sender := <-done
	if started, joined, interrupted := session.CleanupCompletion(); !started || !joined || !interrupted {
		t.Fatal("fixture did not join its real source owner")
	}
	result := sourceCaptureFinal(rejected, inputdesktop.ErrInactive, sender, false)
	if result == nil || !errors.Is(result, rejected) || errors.Is(result, ErrRecovery) || sourceRetirementStatus(result) != 0 {
		t.Fatal("pre-native state rejection became uncertain native cleanup", result)
	}
	var wire bytes.Buffer
	if err := writeReport(&wire, Report{}, result); err != nil {
		t.Fatal(err)
	}
	_, received := readReport(&wire)
	if received == nil || serviceRecoveryFailure(received, nil) != nil {
		t.Fatal("failed activation was erased or stopped the service", received)
	}
	for _, failure := range []error{
		sourceCaptureFinal(rejected, inputdesktop.ErrCleanup, sender, false),
		sourceCaptureFinal(rejected, inputdesktop.ErrInactive, wininput.ErrSourceCleanupIncomplete, false),
		sourceCaptureFinal(&sourceActivationRejected{cause: inputdesktop.ErrCleanup}, inputdesktop.ErrInactive, sender, false),
		sourceCaptureFinal(rejected, inputdesktop.ErrInactive, rejected, false),
	} {
		if !errors.Is(failure, ErrRecovery) || sourceRetirementStatus(failure) != 1 {
			t.Fatal("state rejection hid an independent native/sender cleanup failure", failure)
		}
	}
}
