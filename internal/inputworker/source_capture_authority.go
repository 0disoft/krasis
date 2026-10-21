package inputworker

import "context"

func sourceCaptureAuthorization(ctx context.Context, service, agent *Channel) func() error {
	return func() error {
		// Capture/desktop admission always needs the full proof, under the
		// same owned-query budget as a full authenticated source command.
		return authorizeSourceRead(ctx, service, agent, sourceAdmit)
	}
}
