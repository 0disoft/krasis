//go:build windows && desktop

package workspacehost

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/0disoft/krasis/internal/releaseupdate"
	"github.com/0disoft/krasis/internal/securefs"
	"github.com/0disoft/krasis/internal/updatebundle"
)

type uiReplacement struct {
	gate                sync.Mutex
	directory, revision string
	generation          int
}

// Only a complete newer product bundle is staged. IPC arguments never become
// executable arguments; the existing verified updater owns the actual launch.
func (s *WorkspaceService) requestUIReplacement(args []string, data map[string]string) bool {
	generation, err := strconv.Atoi(data["generation"])
	if err != nil || generation <= localapi.AgentGeneration || len(args) == 0 || !filepath.IsAbs(args[0]) || filepath.Base(args[0]) != "krasis.exe" {
		return false
	}
	if s.ctx.Err() != nil || !s.uiReplacement.gate.TryLock() {
		return true
	}
	defer s.uiReplacement.gate.Unlock()
	if s.uiReplacement.directory != "" {
		return true
	}
	guard, err := securefs.OpenReadDirectory(filepath.Dir(args[0]))
	if err != nil {
		return true
	}
	defer guard.Close()
	archive, err := updatebundle.Pack(guard)
	if err != nil {
		s.log.Event("ui_replacement_bundle_failed")
		return true
	}
	bundle, err := updatebundle.Read(bytes.NewReader(archive), int64(len(archive)))
	if err != nil || bundle.Metadata().Generation != generation || bundle.Metadata().Revision != data["revision"] {
		s.log.Event("ui_replacement_bundle_failed")
		return true
	}
	directory, err := releaseupdate.Stage(os.Getenv("LOCALAPPDATA"), bundle)
	if err != nil {
		s.log.Event("ui_replacement_stage_failed")
		return true
	}
	s.uiReplacement.directory, s.uiReplacement.revision, s.uiReplacement.generation = directory, bundle.Metadata().Revision, generation
	s.log.Event("ui_replacement_waiting_for_idle")
	if window := s.visibleWindow.Load(); window != nil {
		window.SetTitle("Krasis · g" + strconv.Itoa(localapi.AgentGeneration) + " · 새 버전 대기")
	}
	return true
}

func (s *WorkspaceService) runUIReplacement() bool {
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return false
		case <-tick.C:
			s.uiReplacement.gate.Lock()
			directory, revision, generation := s.uiReplacement.directory, s.uiReplacement.revision, s.uiReplacement.generation
			s.uiReplacement.gate.Unlock()
			if directory == "" {
				continue
			}
			failed := false
			if s.managed.ApplyReleaseUpdate(generation, revision, func() error {
				err := productpanel.LaunchReleaseWorkspaceUpdate(directory, revision)
				failed = err != nil
				return err
			}) {
				return true
			}
			if failed {
				s.log.Event("ui_replacement_launch_failed")
				s.uiReplacement.gate.Lock()
				s.uiReplacement.directory = ""
				s.uiReplacement.gate.Unlock()
			}
		}
	}
}
