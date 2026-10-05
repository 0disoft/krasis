package agent

import (
	"context"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
)

type readDetailConnection interface {
	SetReadDetailObserver(func(int, time.Time)) bool
}

func configureGrantReadProgress(ctx context.Context, connection Connection) {
	bytes := false
	if source, ok := connection.(readDetailConnection); ok {
		bytes = source.SetReadDetailObserver(func(count int, at time.Time) {
			handoff.ReadProgressBytes(ctx, count, at)
		})
	}
	handoff.StartGrantProgress(ctx, bytes, time.Now())
}

func (connection *supervisedConnection) SetReadDetailObserver(observer func(int, time.Time)) bool {
	source, ok := connection.Connection.(readDetailConnection)
	return ok && source.SetReadDetailObserver(observer)
}
