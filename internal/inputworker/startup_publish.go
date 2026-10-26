package inputworker

import (
	"context"
	"io"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type startupPublisher interface {
	io.Writer
	SetWriteDeadline(time.Time) error
}

// Failure remains failure even if the worker later supplies a clean recovery
// receipt. The caller still owns shutdown and consumption of that receipt.
func publishStartupReady(ctx context.Context, agent *Channel, expires time.Time, profile handoff.StartupProfile) error {
	if agent == nil {
		return ErrAuthority
	}
	return publishAuthorizedReady(ctx, agent, authorityAgent, agent.stream, expires, profile)
}
