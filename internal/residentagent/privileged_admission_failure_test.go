package residentagent

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
)

type failedAdmissionClient struct {
	testPrivilegedClient
	closeFailure error
}

func (c *failedAdmissionClient) Close() error { c.closes.Add(1); return c.closeFailure }

func TestPrivilegedAdmissionPreservesOriginalFailureAndCleanup(t *testing.T) {
	for _, partial := range []bool{false, true} {
		t.Run(map[bool]string{false: "connect", true: "partial-client"}[partial], func(t *testing.T) {
			original := syscall.Errno(109)
			cleanup := errors.New("private-cleanup-path")
			client := &failedAdmissionClient{closeFailure: cleanup}
			var events []string
			input := &privilegedLeaseInput{event: func(s string) { events = append(events, s) }, connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
				if partial {
					return client, original
				}
				return nil, original
			}}
			err := input.open(context.Background(), application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now())
			if !errors.Is(err, errPrivilegedInput) || !errors.Is(err, original) || input.client != nil {
				t.Fatal("original failure lost or channel published", err)
			}
			if partial && (!errors.Is(err, cleanup) || client.closes.Load() != 1 || !errors.Is(input.closeErr, cleanup)) {
				t.Fatal("cleanup failure lost", err)
			}
			for _, event := range events {
				if strings.Contains(event, "private") {
					t.Fatal("private error leaked", event)
				}
			}
		})
	}
}

func TestPrivilegedAdmissionIdentifiesExistingOwnerWithoutTouchingIt(t *testing.T) {
	client := &testPrivilegedClient{}
	var events []string
	input := &privilegedLeaseInput{client: client, connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
		t.Fatal("opened over an existing owner")
		return nil, nil
	}, event: func(s string) { events = append(events, s) }}
	if err := input.open(context.Background(), application.LeaseIdentity{SessionID: "s", LeaseID: "l", Epoch: "e"}, time.Now()); !errors.Is(err, errPrivilegedInput) || input.client != client || client.closes.Load() != 0 {
		t.Fatal("existing owner changed", err)
	}
	for _, event := range events {
		if event == "installed_input_admission_blocked_active" {
			return
		}
	}
	t.Fatal("existing owner rejection lost its stage", events)
}
