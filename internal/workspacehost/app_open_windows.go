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
	s.uiReadyEpoch.Add(1)
}

// Re-report after each show request and after an agent replacement. Installation
// delays never permanently consume the opportunity to confirm a visible UI.
func (s *WorkspaceService) runUIReadyReporter() {
	ctx := s.ctx
	tick := time.NewTicker(time.Second)
	defer tick.Stop()
	var reportedInstance string
	var reportedEpoch uint64
	for {
		window := s.visibleWindow.Load()
		epoch := s.uiReadyEpoch.Load()
		if window != nil && epoch != 0 && window.IsVisible() {
			c, err := localapi.OpenClient(s.control)
			attempt, done := context.WithTimeout(ctx, time.Second)
			if err == nil {
				info, runtimeErr := c.Runtime(attempt)
				if runtimeErr == nil && info.Instance != "" && (reportedInstance != info.Instance || reportedEpoch != epoch) {
					result, readyErr := c.UIReady(attempt, uint32(os.Getpid()))
					if readyErr == nil && result.Code == "ui_ready" {
						reportedInstance, reportedEpoch = info.Instance, epoch
					}
				}
			}
			done()
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}
