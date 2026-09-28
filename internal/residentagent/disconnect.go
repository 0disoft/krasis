package residentagent

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/agent"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/transport"
)

var errLocalDisconnect = errors.New("local disconnect requested")

// Only the authenticated product connection carries user-stop intent. EOF,
// trust expiry, crashes and lifecycle cancellation keep their existing policy.
type disconnectSource struct {
	agent.ConnectionSource
	intent context.Context
	log    *productlog.Logger
}

func (source *disconnectSource) Next(ctx context.Context) (agent.Connection, error) {
	connection, err := source.ConnectionSource.Next(ctx)
	if err != nil || connection == nil {
		return connection, err
	}
	if policy, ok := connection.(interface{ KeepAlivePolicy() string }); ok {
		source.log.Event(policy.KeepAlivePolicy())
	}
	return &disconnectConnection{Connection: connection, intent: source.intent, log: source.log}, nil
}

type disconnectConnection struct {
	agent.Connection
	intent context.Context
	log    *productlog.Logger
	once   sync.Once
	err    error
}

func (connection *disconnectConnection) DiagnosticID() string {
	if diagnostic, ok := connection.Connection.(interface{ DiagnosticID() string }); ok {
		return diagnostic.DiagnosticID()
	}
	return ""
}

func (connection *disconnectConnection) SetReadProgressObserver(observer func(int, time.Time)) {
	if source, ok := connection.Connection.(interface{ SetReadProgressObserver(func(int, time.Time)) }); ok {
		source.SetReadProgressObserver(observer)
	}
}

func (connection *disconnectConnection) Close() error {
	connection.once.Do(func() {
		if errors.Is(context.Cause(connection.intent), errLocalDisconnect) {
			// The transport write gate serializes this final frame with any
			// in-flight input. A lost peer must never hold up local release.
			ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
			if err := connection.Connection.WriteFrameContext(ctx, transport.Frame{Kind: transport.MessageDisconnect}); err != nil {
				connection.log.Event("disconnect_notification_failed")
			}
			cancel()
		}
		connection.err = connection.Connection.Close()
	})
	return connection.err
}
