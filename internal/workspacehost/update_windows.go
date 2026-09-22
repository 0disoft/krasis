//go:build windows && desktop

package workspacehost

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime/debug"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productpanel"
	"golang.org/x/sys/windows"
)

func updateBundleDirectory(self string) string {
	if filepath.Base(self) == "krasis-control.exe" {
		return filepath.Dir(self)
	}
	return filepath.Join(filepath.Dir(self), "peer-update")
}

func prepareUpdatedWorkspace(ctx context.Context, control string, startup productpanel.Startup, launch productpanel.WorkspaceUpdateLaunch) error {
	revision := ""
	if info, ok := debug.ReadBuildInfo(); ok {
		for _, s := range info.Settings {
			if s.Key == "vcs.revision" {
				revision = s.Value
			}
		}
	}
	if revision == "" || revision != launch.Revision {
		return errors.New("unexpected Workspace build")
	}
	if err := waitUpdateParent(launch.WaitPID); err != nil {
		return err
	}
	if err := startup.Ensure(ctx); err != nil {
		return err
	}
	c, err := localapi.OpenClient(control)
	if err != nil {
		return err
	}
	// Reconnect is best-effort: a temporary peer outage must not hide the new UI.
	_ = productpanel.ResumeUpdatedWorkspace(ctx, c, launch)
	return nil
}

// Waiting is read-only and bounded. It avoids the old Workspace single-instance
// handler swallowing the replacement process before the old UI has quit.
func waitUpdateParent(pid uint32) error {
	if pid == 0 {
		return nil
	}
	if pid == uint32(os.Getpid()) {
		return errors.New("cannot wait on self")
	}
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, pid)
	if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
		return nil
	}
	if err != nil {
		return err
	}
	defer windows.CloseHandle(h)
	result, err := windows.WaitForSingleObject(h, 15000)
	if err != nil {
		return err
	}
	if result != windows.WAIT_OBJECT_0 {
		return errors.New("previous Workspace did not exit")
	}
	return nil
}

func (s *WorkspaceService) runUpdateReceiver() {
	handoff := false
	defer func() {
		s.jobs.Done()
		if handoff {
			s.quit()
		}
	}()
	timer := time.NewTicker(1500 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-timer.C:
			if s.managed.ApplyReceivedUpdate(s.receiver) {
				handoff = true
				return
			}
		}
	}
}
