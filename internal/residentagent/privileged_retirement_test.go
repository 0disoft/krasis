package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

func TestPlannedLeaseReturnBeforeFiftyFiveMinutesAllowsReentry(t *testing.T) {
	now := time.Now()
	input := &privilegedLeaseInput{expires: now.Add(2 * time.Hour), connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
		return &testPrivilegedClient{}, nil
	}}
	defer input.close(nil)
	id := application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}
	if err := input.open(context.Background(), id, now); err != nil {
		t.Fatal(err)
	}
	calls := 0
	retire := func(_ context.Context, got application.LeaseIdentity) error {
		if got != id {
			t.Fatal(got)
		}
		calls++
		input.close(nil)
		return nil
	}
	if err := input.retireDue(context.Background(), now.Add(54*time.Minute), retire); err != nil || calls != 0 {
		t.Fatal(err, calls)
	}
	if err := input.retireDue(context.Background(), now.Add(55*time.Minute-10*time.Second), retire); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
	if err := input.retireDue(context.Background(), now.Add(55*time.Minute+time.Second), retire); err != nil || calls != 1 {
		t.Fatal(err, calls)
	}
	if err := input.open(context.Background(), id, now.Add(55*time.Minute+time.Second)); err != nil {
		t.Fatal("fresh lease unavailable", err)
	}
	failure := errors.New("release failed")
	if err := input.retireDue(context.Background(), now.Add(110*time.Minute), func(context.Context, application.LeaseIdentity) error { return failure }); !errors.Is(err, errPrivilegedInput) || !errors.Is(err, failure) {
		t.Fatal("failed retirement hidden", err)
	}
}
