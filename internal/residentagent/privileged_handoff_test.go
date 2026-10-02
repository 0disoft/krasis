package residentagent

import (
	"context"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/control"
	"github.com/0disoft/krasis/internal/handoff"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPrivilegedHandoffRecordsConnectSeparatelyFromFirstUncertainInput(t *testing.T) {
	var summaries []handoff.Summary
	ctx := handoff.WithSession(context.Background(), handoff.New(func(s handoff.Summary) { summaries = append(summaries, s) }))
	p := handoff.Begin(ctx, "receiver", "session", "epoch", time.Now())
	client := &testPrivilegedClient{err: &application.InjectionError{Certainty: application.InjectionOutcomeUnknown}}
	input := &privilegedLeaseInput{expires: time.Now().Add(time.Hour), connect: func(ctx context.Context, _ application.LeaseIdentity, _ time.Time) (privilegedClient, error) {
		time.Sleep(20 * time.Millisecond)
		return client, nil
	}}
	authorizer := privilegedAuthorizer{LeaseAuthorizer: testPrivilegedAuthorizer{accepted: true}, input: input}
	if _, accepted, err := authorizer.AuthorizeLease(p, transport.AuthenticatedPeerContext{}, "session", "epoch", time.Now()); err != nil || !accepted {
		t.Fatal("approved connection failed")
	}
	actual, complete := handoff.Input(ctx, "receiver")
	err := input.Press(actual, control.PressedToken{})
	complete(err)
	if err != client.err || client.presses.Load() != 1 {
		t.Fatal("profile changed input outcome or replayed")
	}
	input.close(nil)
	handoff.Finish(p, "error")
	if len(summaries) != 1 {
		t.Fatal("missing installed profile")
	}
	s := summaries[0]
	if !s.Valid() || s.Backend != "installed" || s.Preparation.Micros[handoff.ConnectTotal] < 20000 || s.First.Outcome != "error" {
		t.Fatalf("connect/first-input lost: %+v", s)
	}
	for _, stage := range []handoff.Stage{handoff.PeerAuthorize, handoff.OpenLock, handoff.InputClose} {
		if s.Preparation.Measured&(1<<stage) == 0 {
			t.Fatalf("missing preparation %d", stage)
		}
	}
	if s.First.Measured&(1<<handoff.OuterLock) == 0 || s.First.Measured&(1<<handoff.ClientTotal) == 0 {
		t.Fatal("missing first-call vector")
	}
}
