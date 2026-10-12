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
	input := &privilegedLeaseInput{expires: expires, failed: failed, dispose: service.Close, prepare: service.PrepareConsole, connect: func(ctx context.Context, id application.LeaseIdentity, expires time.Time) (privilegedClient, error) {
		binding, err := inputworker.NewBinding()
		if err != nil {
			return nil, err
		}
		return service.ConnectLease(ctx, inputworker.Lease{Identity: id, Binding: binding, ExpiresAt: expires})
	}}
	input.cursor.open = func(ctx context.Context, lease inputworker.Lease) (func() error, error) {
		return service.PresentReturnCursorObserved(ctx, lease, input.event)
	}
	input.source = service.NewSourceCapture()
	return input
}
