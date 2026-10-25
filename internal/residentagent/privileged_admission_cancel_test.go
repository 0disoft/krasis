package residentagent

import (
	"context"
	"errors"
	"testing"
	"testing/synctest"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPrivilegedAdmissionRejectsConnectionCompletingAfterCancellation(t *testing.T) {
	for _, mode := range []string{"cancel", "deadline", "cleanup-failure"} {
		t.Run(mode, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				ctx, cancel := context.WithCancel(context.Background())
				want := context.Canceled
				if mode == "deadline" {
					cancel()
					ctx, cancel = context.WithTimeout(context.Background(), time.Millisecond)
					want = context.DeadlineExceeded
				}
				defer cancel()
				cleanup := errors.New("worker cleanup failed")
				candidate := &failedAdmissionClient{}
				if mode == "cleanup-failure" {
					candidate.closeFailure = cleanup
				}
				fresh := &testPrivilegedClient{}
				connects, ready := 0, 0
				input := &privilegedLeaseInput{event: func(code string) {
					if code == "installed_input_lease_ready" {
						ready++
					}
				}, connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
					connects++
					if connects != 1 {
						return fresh, nil
					}
					// Model completion racing cancellation after the connector's last check.
					if mode == "deadline" {
						time.Sleep(time.Millisecond + time.Nanosecond)
					} else {
						cancel()
					}
					return candidate, nil
				}}
				defer input.shutdown()
				authorizer := privilegedAuthorizer{LeaseAuthorizer: testPrivilegedAuthorizer{accepted: true}, input: input}
				id, accepted, err := authorizer.AuthorizeLease(ctx, transport.AuthenticatedPeerContext{}, "s", "e", time.Now())
				if id != "" || accepted || !errors.Is(err, errPrivilegedInput) || !errors.Is(err, want) {
					t.Error("canceled entry granted a lease or lost its cancellation cause", id, accepted, err)
				}
				if candidate.closes.Load() != 1 || input.client != nil || input.done != nil || input.cancel != nil || input.profile != nil || input.identity != (application.LeaseIdentity{}) || candidate.observe != nil || ready != 0 {
					t.Error("late channel was not retired before input/heartbeat publication", candidate.closes.Load(), ready)
				}
				if mode == "cleanup-failure" {
					if !errors.Is(err, cleanup) || !errors.Is(input.closeErr, cleanup) {
						t.Error("late-channel cleanup failure was lost", err)
					}
					if retryErr := input.open(context.Background(), application.LeaseIdentity{SessionID: "s", LeaseID: "next", Epoch: "e"}, time.Now()); !errors.Is(retryErr, cleanup) || connects != 1 {
						t.Error("unconfirmed cleanup allowed another worker", retryErr, connects)
					}
				} else {
					if id, accepted, err := authorizer.AuthorizeLease(context.Background(), transport.AuthenticatedPeerContext{}, "s", "next", time.Now()); err != nil || !accepted || id == "" || input.client != fresh || connects != 2 {
						t.Error("clean rejection blocked a fresh entry", id, accepted, err, connects)
					}
				}
			})
		})
	}
}
