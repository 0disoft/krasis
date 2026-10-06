//go:build windows && desktop

package workspacehost

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/0disoft/krasis/internal/releaseupdate"
	"os"
	"sync"
	"time"
)

type ReleaseSnapshot struct {
	State      string `json:"state"`
	Generation int    `json:"generation"`
}
type releaseState struct {
	gate                sync.Mutex
	value               ReleaseSnapshot
	directory, revision string
	wake                chan struct{}
	cancel              context.CancelFunc
}

func (s *WorkspaceService) ReleaseStatus() ReleaseSnapshot {
	s.release.gate.Lock()
	defer s.release.gate.Unlock()
	return s.release.value
}
func (s *WorkspaceService) CheckRelease() ReleaseSnapshot {
	if s.release.wake != nil {
		select {
		case s.release.wake <- struct{}{}:
		default:
		}
	}
	return s.ReleaseStatus()
}
func (s *WorkspaceService) setRelease(state string, generation int) {
	s.release.gate.Lock()
	s.release.value = ReleaseSnapshot{state, generation}
	s.release.gate.Unlock()
}
func (s *WorkspaceService) checkRelease() {
	parent, cancel := context.WithTimeout(s.ctx, 3*time.Minute)
	defer cancel()
	s.release.gate.Lock()
	s.release.cancel = cancel
	s.release.gate.Unlock()
	defer func() { s.release.gate.Lock(); s.release.cancel = nil; s.release.gate.Unlock() }()
	s.setRelease("CHECKING", 0)
	m, err := releaseupdate.Check(parent, releaseupdate.Client(), releaseupdate.PublicKey)
	if err != nil {
		if errors.Is(err, releaseupdate.ErrUnconfigured) {
			s.setRelease("UNCONFIGURED", 0)
		} else {
			s.setRelease("FAILED", 0)
		}
		return
	}
	if m.Generation <= localapi.AgentGeneration {
		s.setRelease("CURRENT", 0)
		return
	}
	s.setRelease("AVAILABLE", m.Generation)
	settings, err := appsettings.Load(os.Getenv("LOCALAPPDATA"))
	if err != nil || !settings.AutoUpdate {
		return
	}
	s.setRelease("DOWNLOADING", m.Generation)
	bundle, err := releaseupdate.Download(parent, releaseupdate.Client(), m)
	if err != nil {
		s.setRelease("FAILED", m.Generation)
		return
	}
	settings, err = appsettings.Load(os.Getenv("LOCALAPPDATA"))
	if parent.Err() != nil || err != nil || !settings.AutoUpdate {
		s.setRelease("AVAILABLE", m.Generation)
		return
	}
	directory, err := releaseupdate.Stage(os.Getenv("LOCALAPPDATA"), bundle)
	if err != nil {
		s.setRelease("FAILED", m.Generation)
		return
	}
	s.release.gate.Lock()
	s.release.directory, s.release.revision = directory, m.Revision
	s.release.value = ReleaseSnapshot{"READY", m.Generation}
	s.release.gate.Unlock()
}
func (s *WorkspaceService) runReleaseUpdater() {
	applied := false
	defer func() {
		s.jobs.Done()
		if applied {
			s.quit()
		}
	}()
	check := time.NewTimer(time.Minute)
	defer check.Stop()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-check.C:
			settings, err := appsettings.Load(os.Getenv("LOCALAPPDATA"))
			if err == nil && settings.AutoUpdate && s.ReleaseStatus().State != "READY" {
				s.checkRelease()
			}
			check.Reset(6 * time.Hour)
		case <-s.release.wake:
			if s.ReleaseStatus().State != "READY" {
				s.checkRelease()
			}
			if !check.Stop() {
				select {
				case <-check.C:
				default:
				}
			}
			check.Reset(6 * time.Hour)
		case <-tick.C:
			if s.ReleaseStatus().State != "READY" {
				continue
			}
			settings, err := appsettings.Load(os.Getenv("LOCALAPPDATA"))
			if err != nil || !settings.AutoUpdate {
				continue
			}
			pairing := s.pairs.Status()
			if pairing.Code != "ok" || pairing.Job != "" {
				continue
			}
			s.release.gate.Lock()
			directory, revision := s.release.directory, s.release.revision
			s.release.gate.Unlock()
			launchFailed := false
			applied = s.managed.ApplyReleaseUpdate(func() error {
				err := productpanel.LaunchReleaseWorkspaceUpdate(directory, revision)
				launchFailed = err != nil
				return err
			})
			if launchFailed {
				s.setRelease("FAILED", s.ReleaseStatus().Generation)
			}
			if applied {
				return
			}
		}
	}
}
