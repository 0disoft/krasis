//go:build windows && desktop

package workspacehost

import (
	"os"

	"github.com/0disoft/krasis/internal/productpanel"
)

func (s *WorkspaceService) scheduledRecoverAsync() {
	if s.managed == nil || s.ctx.Err() != nil {
		return
	}
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		s.managed.ScheduledRecover(func() bool { return productpanel.RecoveryAllowed(os.Getenv("LOCALAPPDATA")) }, s.resumeSavedConnection)
	}()
}
