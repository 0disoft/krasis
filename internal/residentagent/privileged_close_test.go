package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

type closeFailureClient struct {
	testPrivilegedClient
	failure error
}

func (c *closeFailureClient) Close() error {
	c.closes.Add(1)
	return c.failure
}

func TestPrivilegedCloseFailureFencesReentryAndSurvivesShutdown(t *testing.T) {
	closeFailure, disposeFailure := errors.New("channel close failed"), errors.New("pin close failed")
	client := &closeFailureClient{failure: closeFailure}
	opens := 0
	input := &privilegedLeaseInput{
		connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
			opens++
			return client, nil
		},
		dispose: func() error { return disposeFailure },
	}
	identity := application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}
	if err := input.open(context.Background(), identity, time.Now()); err != nil {
		t.Fatal(err)
	}
	input.close(nil)
	if err := input.open(context.Background(), identity, time.Now()); !errors.Is(err, closeFailure) || opens != 1 {
		t.Fatal("failed close allowed reentry", err, opens)
	}
	for i := 0; i < 2; i++ {
		if err := input.shutdown(); !errors.Is(err, closeFailure) || !errors.Is(err, disposeFailure) {
			t.Fatal("shutdown discarded close failure", err)
		}
	}
	if client.closes.Load() != 1 {
		t.Fatal("client retired twice")
	}
}
