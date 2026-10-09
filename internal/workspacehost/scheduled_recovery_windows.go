//go:build windows && desktop

package workspacehost

import (
	"github.com/0disoft/krasis/internal/appsettings"
	"os"

	"github.com/0disoft/krasis/internal/productpanel"
)

func (s *WorkspaceService) scheduledRecoverAsync(token string) {
	if s.managed == nil || s.ctx.Err() != nil {
		return
	}
	s.jobs.Add(1)
	go func() {
		defer s.jobs.Done()
		startup, err := productpanel.NewStartup(s.control, token)
		if err != nil {
			return
		}
		s.managed.ScheduledRecover(func() bool { return appsettings.RecoveryCurrent(os.Getenv("LOCALAPPDATA"), token) }, startup.Ensure, s.resumeSavedConnection)
	}()
}
