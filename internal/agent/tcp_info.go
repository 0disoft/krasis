package agent

import (
	"context"

	"github.com/0disoft/krasis/internal/handoff"
)

type tcpInfoConnection interface {
	DiagnosticTCPInfo() ([4]uint32, bool)
}

func configureTCPInfo(ctx context.Context, connection Connection) {
	if query, ok := connection.(tcpInfoConnection); ok {
		handoff.ConfigureTCPInfo(ctx, query.DiagnosticTCPInfo)
	}
}

func (connection *supervisedConnection) DiagnosticTCPInfo() ([4]uint32, bool) {
	if connection != nil {
		if query, ok := connection.Connection.(tcpInfoConnection); ok {
			return query.DiagnosticTCPInfo()
		}
	}
	return [4]uint32{}, false
}
