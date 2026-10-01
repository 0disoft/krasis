//go:build windows

package residentagent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputworker"
)

func installedProductInput(expires time.Time, failed func(error)) *privilegedLeaseInput {
	digest, err := inputworker.CurrentAgentServiceDigest()
	if err != nil {
		return nil
	}
	return &privilegedLeaseInput{expires: expires, failed: failed, connect: func(ctx context.Context, id application.LeaseIdentity, expires time.Time) (privilegedClient, error) {
		binding, err := inputworker.NewBinding()
		if err != nil {
			return nil, err
		}
		return inputworker.ConnectLease(ctx, digest, inputworker.Lease{Identity: id, Binding: binding, ExpiresAt: expires})
	}}
}
