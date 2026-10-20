package inputworker

import (
	"context"
	"time"
)

// A service-owned endpoint waits for a client until service shutdown. Starting
// a fresh timeout while idle periodically cancels a valid listening instance,
// leaving a client able to connect just before the serial loop resets it.
// Single-use accepts retain their bound; lease reads have a separate deadline.
func agentAcceptContext(parent context.Context, serviceOwned bool) (context.Context, context.CancelFunc) {
	if serviceOwned {
		return context.WithCancel(parent)
	}
	return context.WithTimeout(parent, 5*time.Second)
}
