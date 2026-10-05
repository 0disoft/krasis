package agent

import (
	"context"

	"github.com/0disoft/krasis/internal/handoff"
)

type handoffCorrelationConnection interface {
	DiagnosticExchangeTag(string, string) string
}

func configureHandoffCorrelation(ctx context.Context, connection Connection) {
	if source, ok := connection.(handoffCorrelationConnection); ok {
		handoff.ConfigureExchangeTags(ctx, source.DiagnosticExchangeTag)
	}
}

func (connection *supervisedConnection) DiagnosticExchangeTag(sessionID, epoch string) string {
	if connection == nil {
		return ""
	}
	if source, ok := connection.Connection.(handoffCorrelationConnection); ok {
		return source.DiagnosticExchangeTag(sessionID, epoch)
	}
	return ""
}
