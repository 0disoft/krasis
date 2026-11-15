package agent

import (
	"context"
	"github.com/0disoft/krasis/internal/handoff"
)

type receiveQueueConnection interface{ DiagnosticReceiveQueue() (uint32, bool) }

func configureSocketReceiveProbe(ctx context.Context, connection Connection) func() {
	if source, ok := connection.(receiveQueueConnection); ok {
		return handoff.ConfigureSocketReceiveProbe(ctx, source.DiagnosticReceiveQueue)
	}
	return func() {}
}

func (connection *supervisedConnection) DiagnosticReceiveQueue() (uint32, bool) {
	if connection != nil {
		if source, ok := connection.Connection.(receiveQueueConnection); ok {
			return source.DiagnosticReceiveQueue()
		}
	}
	return 0, false
}
