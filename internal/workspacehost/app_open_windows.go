//go:build windows && desktop

package workspacehost

import (
	"context"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productpanel"
	"os"
	"time"
)

func (s *WorkspaceService) PeerAppOpen(peer, id string) productpanel.AppOpenSnapshot {
	return s.updates.AppOpen(peer, id)
}

func (s *WorkspaceService) reportUIReady() {
	ctx, cancel := context.WithTimeout(s.ctx, 15*time.Second)
	defer cancel()
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	for {
		if c, err := localapi.OpenClient(s.control); err == nil {
			attempt, done := context.WithTimeout(ctx, time.Second)
			result, err := c.UIReady(attempt, uint32(os.Getpid()))
			done()
			if err == nil && result.Code == "ui_ready" {
				return
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
