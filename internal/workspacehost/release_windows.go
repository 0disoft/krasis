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
	checkPending        bool
	cancelledByUser     bool
	recheckAfterCancel  bool
}

func (s *WorkspaceService) ReleaseStatus() ReleaseSnapshot {
	s.release.gate.Lock()
	defer s.release.gate.Unlock()
	if s.release.checkPending || s.release.recheckAfterCancel {
		return ReleaseSnapshot{State: "CHECKING"}
	}
	return s.release.value
}
func (s *WorkspaceService) CheckRelease() ReleaseSnapshot {
	s.release.gate.Lock()
	defer s.release.gate.Unlock()
	if s.release.wake == nil {
		return ReleaseSnapshot{State: "FAILED"}
	}
	if s.release.cancelledByUser && (s.release.checkPending || s.release.cancel != nil || s.release.value.State == "CHECKING" || s.release.value.State == "DOWNLOADING") {
		// The canceled context cannot be revived. Keep one explicit follow-up
		// until this worker finishes, instead of coalescing it into canceled work.
		s.release.recheckAfterCancel = true
		return ReleaseSnapshot{State: "CHECKING"}
	}
	if s.release.checkPending {
		return ReleaseSnapshot{State: "CHECKING"}
	}
	if s.release.value.State == "CHECKING" || s.release.value.State == "DOWNLOADING" {
		return s.release.value
	}
	select {
	case s.release.wake <- struct{}{}:
	default:
	}
	s.release.checkPending = true
	s.release.cancelledByUser = false
	return ReleaseSnapshot{State: "CHECKING"}
}
func (s *WorkspaceService) setRelease(state string, generation int) {
	s.release.gate.Lock()
	s.release.value = ReleaseSnapshot{state, generation}
	s.release.gate.Unlock()
}
func (s *WorkspaceService) checkRelease() bool {
	s.release.gate.Lock()
	previous := s.release.value
	if !s.release.checkPending {
		s.release.cancelledByUser = false
	}
	s.release.checkPending = false
	select {
	case <-s.release.wake:
	default:
	}
	s.release.value = ReleaseSnapshot{State: "CHECKING"}
	s.release.gate.Unlock()
	fail := func(err error, generation int) bool { return s.finishReleaseError(err, previous, generation) }
	parent, cancel := context.WithTimeout(s.ctx, 3*time.Minute)
	defer cancel()
	s.release.gate.Lock()
	s.release.cancel = cancel
	if s.release.cancelledByUser {
		cancel()
	}
	s.release.gate.Unlock()
	defer s.finishReleaseCheck()
	if parent.Err() != nil {
		return fail(parent.Err(), 0)
	}
	m, envelope, err := releaseupdate.CheckSigned(parent, releaseupdate.Client(), releaseupdate.PublicKey)
	if err != nil {
		if errors.Is(err, releaseupdate.ErrUnconfigured) {
			s.setRelease("UNCONFIGURED", 0)
			return true
		}
		return fail(err, 0)
	}
	if previous.State == "READY" && m.Generation <= previous.Generation {
		s.setRelease("READY", previous.Generation)
		return true
	}
	if m.Generation <= localapi.AgentGeneration {
		s.setRelease("CURRENT", 0)
		return true
	}
	s.setRelease("AVAILABLE", m.Generation)
	settings, err := appsettings.Load(os.Getenv("LOCALAPPDATA"))
	if err != nil || !settings.AutoUpdate {
		return true
	}
	s.setRelease("DOWNLOADING", m.Generation)
	_, archive, err := releaseupdate.DownloadArchive(parent, releaseupdate.Client(), m)
	if err != nil {
		return fail(err, m.Generation)
	}
	if parent.Err() != nil {
		return fail(parent.Err(), m.Generation)
	}
	settings, err = appsettings.Load(os.Getenv("LOCALAPPDATA"))
	if err != nil || !settings.AutoUpdate {
		s.setRelease("AVAILABLE", m.Generation)
		return true
	}
	directory, _, err := releaseupdate.SaveReady(os.Getenv("LOCALAPPDATA"), envelope, archive, releaseupdate.PublicKey)
	if err != nil {
		return fail(err, m.Generation)
	}
	s.release.gate.Lock()
	s.release.directory, s.release.revision = directory, m.Revision
	s.release.value = ReleaseSnapshot{"READY", m.Generation}
	cancelled := s.release.cancelledByUser
	if cancelled {
		s.release.value.State = "CANCELLED"
	}
	s.release.gate.Unlock()
	if cancelled {
		s.log.Event("release_check_cancelled")
	}
	if err := releaseupdate.PruneReady(os.Getenv("LOCALAPPDATA"), releaseupdate.PublicKey); err != nil {
		s.log.Event("release_cache_cleanup_failed")
	}
	return true
}

func (s *WorkspaceService) finishReleaseCheck() {
	s.release.gate.Lock()
	defer s.release.gate.Unlock()
	s.release.cancel = nil
	if !s.release.recheckAfterCancel {
		return
	}
	s.release.recheckAfterCancel = false
	if s.ctx.Err() != nil {
		return
	}
	s.release.cancelledByUser = false
	s.release.checkPending = true
	select {
	case s.release.wake <- struct{}{}:
	default:
	}
}

func (s *WorkspaceService) finishReleaseError(err error, previous ReleaseSnapshot, generation int) bool {
	s.release.gate.Lock()
	userCancelled := s.release.cancelledByUser
	s.release.gate.Unlock()
	if userCancelled && errors.Is(err, context.Canceled) {
		s.setRelease("CANCELLED", generation)
		s.log.Event("release_check_cancelled")
		return true
	}
	if previous.State == "READY" {
		s.setRelease("READY", previous.Generation)
	} else {
		s.setRelease("FAILED", generation)
	}
	s.log.Event("release_check_failed")
	return false
}
func (s *WorkspaceService) runReleaseUpdater() {
	applied := false
	defer func() {
		s.jobs.Done()
		if applied {
			s.quit()
		}
	}()
	base := os.Getenv("LOCALAPPDATA")
	if directory, m, err := releaseupdate.RestoreReady(base, releaseupdate.PublicKey, localapi.AgentGeneration); err == nil {
		s.release.gate.Lock()
		s.release.directory, s.release.revision = directory, m.Revision
		s.release.value = ReleaseSnapshot{"READY", m.Generation}
		s.release.gate.Unlock()
	}
	if err := releaseupdate.PruneReady(base, releaseupdate.PublicKey); err != nil && !errors.Is(err, os.ErrNotExist) {
		s.log.Event("release_cache_cleanup_failed")
	}
	check := time.NewTimer(time.Minute)
	failures := 0
	defer check.Stop()
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-s.ctx.Done():
			return
		case <-check.C:
			settings, err := appsettings.Load(os.Getenv("LOCALAPPDATA"))
			if err == nil && settings.AutoUpdate {
				if s.checkRelease() {
					failures = 0
				} else {
					failures++
				}
			}
			check.Reset(releaseCheckDelay(failures))
		case <-s.release.wake:
			failures = 0
			if !s.checkRelease() {
				failures = 1
			}
			if !check.Stop() {
				select {
				case <-check.C:
				default:
				}
			}
			check.Reset(releaseCheckDelay(failures))
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
			s.settingsGate.Lock()
			settings, err = appsettings.Load(os.Getenv("LOCALAPPDATA"))
			if err != nil || !settings.AutoUpdate {
				s.settingsGate.Unlock()
				continue
			}
			applied = s.managed.ApplyReleaseUpdate(s.ReleaseStatus().Generation, revision, func() error {
				ready, m, err := releaseupdate.RestoreReady(base, releaseupdate.PublicKey, localapi.AgentGeneration)
				if err != nil || ready != directory || m.Revision != revision || m.Generation != s.ReleaseStatus().Generation {
					launchFailed = true
					return releaseupdate.ErrInvalid
				}
				err = productpanel.LaunchReleaseWorkspaceUpdate(directory, revision)
				launchFailed = err != nil
				return err
			})
			s.settingsGate.Unlock()
			if launchFailed {
				s.setRelease("FAILED", s.ReleaseStatus().Generation)
			}
			if applied {
				return
			}
		}
	}
}
