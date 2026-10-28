package residentagent

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputworker"
	"github.com/0disoft/krasis/internal/transport"
)

func TestPrivilegedAuthorizerRejectsOnlyConfirmedStartupRetirement(t *testing.T) {
	for _, mode := range []string{"retired", "recovery", "protocol", "cancelled", "authority"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			input := &privilegedLeaseInput{connect: func(context.Context, application.LeaseIdentity, time.Time) (privilegedClient, error) {
				err := inputworker.ErrStartupRetired
				switch mode {
				case "recovery":
					err = errors.Join(err, inputworker.ErrRecovery)
				case "protocol":
					err = errors.Join(err, inputworker.ErrProtocol)
				case "authority":
					err = inputworker.ErrAuthority
				case "cancelled":
					cancel()
				}
				return nil, err
			}}
			authorizer := privilegedAuthorizer{LeaseAuthorizer: testPrivilegedAuthorizer{accepted: true}, input: input}
			id, accepted, err := authorizer.AuthorizeLease(ctx, transport.AuthenticatedPeerContext{}, "s", "e", time.Now())
			if id != "" || accepted || input.client != nil || (err == nil) != (mode == "retired") {
				t.Fatal("startup retirement granted input or hid uncertain failure", id, accepted, err)
			}
		})
	}
}
