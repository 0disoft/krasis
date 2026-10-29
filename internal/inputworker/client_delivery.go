package inputworker

import "context"

// This is delivery to an already authenticated installed journal, not input
// authorization. Its native child still checks SCM, agent, parent, desktop and
// lease immediately before every injection. Generic clients retain full checks.
// Only connectInstalledLease sets the policy, after full NewClient validation
// and successful startup cancellation join. No diagnostic or wire flag sets it.
func (client *Client) authorizeDelivery(ctx context.Context) error {
	if client.isolatedDelivery {
		if client.channel == nil || client.channel.authority == nil || client.channel.authority.kind != authorityService {
			return ErrAuthority
		}
		return client.channel.authorizePresence(ctx)
	}
	return client.channel.authorizeInput(ctx, true)
}
