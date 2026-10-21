package localapi

import "context"

type runtimeExit interface {
	Wait(context.Context) error
	Close() error
}
