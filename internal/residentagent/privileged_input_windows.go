//go:build windows

package residentagent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/application"
	"github.com/0disoft/krasis/internal/inputworker"
)

func installedProductInput(expires time.Time, failed func(error)) *privilegedLeaseInput {
	service, err := inputworker.OpenAgentService()
	if err != nil {
		return nil
	}
	return &privilegedLeaseInput{cursor: protectedReturnPresentation{open: service.PresentReturnCursor}, expires: expires, failed: failed, dispose: service.Close, prepare: service.PrepareConsole, connect: func(ctx context.Context, id application.LeaseIdentity, expires time.Time) (privilegedClient, error) {
		binding, err := inputworker.NewBinding()
		if err != nil {
			return nil, err
		}
		return service.ConnectLease(ctx, inputworker.Lease{Identity: id, Binding: binding, ExpiresAt: expires})
	}}
}
