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
	State           string `json:"state"`
	Generation      int    `json:"generation"`
	ReadyGeneration int    `json:"readyGeneration"`
}
type releaseState struct {
	gate                sync.Mutex
	value               ReleaseSnapshot
	directory, revision string
	readyGeneration     int
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
		return ReleaseSnapshot{State: "CHECKING", ReadyGeneration: s.release.readyGeneration}
	}
	value := s.release.value
	value.ReadyGeneration = s.release.readyGeneration
	return value
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
		return ReleaseSnapshot{State: "CHECKING", ReadyGeneration: s.release.readyGeneration}
	}
	if s.release.checkPending {
		return ReleaseSnapshot{State: "CHECKING", ReadyGeneration: s.release.readyGeneration}
	}
	if s.release.value.State == "CHECKING" || s.release.value.State == "DOWNLOADING" {
		value := s.release.value
		value.ReadyGeneration = s.release.readyGeneration
		return value
	}
	select {
	case s.release.wake <- struct{}{}:
	default:
	}
	s.release.checkPending = true
	s.release.cancelledByUser = false
	return ReleaseSnapshot{State: "CHECKING", ReadyGeneration: s.release.readyGeneration}
}
func (s *WorkspaceService) setRelease(state string, generation int) {
	s.release.gate.Lock()
	s.release.value = ReleaseSnapshot{State: state, Generation: generation}
	s.release.gate.Unlock()
}
func (s *WorkspaceService) checkRelease() bool {
	s.release.gate.Lock()
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
	fail := func(err error, generation int) bool { return s.finishReleaseError(err, generation) }
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
	s.restoreReleaseReady()
	m, envelope, err := releaseupdate.CheckSigned(parent, releaseupdate.Client(), releaseupdate.PublicKey)
	if err != nil {
		if errors.Is(err, releaseupdate.ErrUnconfigured) {
			s.setRelease("UNCONFIGURED", 0)
			return true
		}
		return fail(err, 0)
	}
	_, _, readyGeneration := s.readyRelease()
	if readyGeneration > 0 && m.Generation <= readyGeneration {
		s.setRelease("READY", readyGeneration)
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
	s.release.readyGeneration = m.Generation
	s.release.value = ReleaseSnapshot{State: "READY", Generation: m.Generation}
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

func (s *WorkspaceService) finishReleaseError(err error, generation int) bool {
	s.release.gate.Lock()
	userCancelled := s.release.cancelledByUser
	s.release.gate.Unlock()
	if userCancelled && errors.Is(err, context.Canceled) {
		s.setRelease("CANCELLED", generation)
		s.log.Event("release_check_cancelled")
		return true
	}
	s.setRelease("FAILED", generation)
	s.log.Event("release_check_failed")
	return false
}

func (s *WorkspaceService) restoreReleaseReady() {
	directory, m, err := releaseupdate.RestoreReady(os.Getenv("LOCALAPPDATA"), releaseupdate.PublicKey, localapi.AgentGeneration)
	s.release.gate.Lock()
	defer s.release.gate.Unlock()
	if err != nil {
		s.release.directory, s.release.revision, s.release.readyGeneration = "", "", 0
		return
	}
	s.release.directory, s.release.revision, s.release.readyGeneration = directory, m.Revision, m.Generation
}

func (s *WorkspaceService) readyRelease() (string, string, int) {
	s.release.gate.Lock()
	defer s.release.gate.Unlock()
	return s.release.directory, s.release.revision, s.release.readyGeneration
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
	s.restoreReleaseReady()
	if _, _, generation := s.readyRelease(); generation > 0 {
		s.setRelease("READY", generation)
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
			status := s.ReleaseStatus()
			if status.State == "CHECKING" || status.State == "DOWNLOADING" || status.ReadyGeneration == 0 {
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
			directory, revision, generation := s.readyRelease()
			launchFailed := false
			s.settingsGate.Lock()
			settings, err = appsettings.Load(os.Getenv("LOCALAPPDATA"))
			if err != nil || !settings.AutoUpdate {
				s.settingsGate.Unlock()
				continue
			}
			applied = s.managed.ApplyReleaseUpdate(generation, revision, func() error {
				ready, m, err := releaseupdate.RestoreReady(base, releaseupdate.PublicKey, localapi.AgentGeneration)
				if err != nil || ready != directory || m.Revision != revision || m.Generation != generation {
					launchFailed = true
					return releaseupdate.ErrInvalid
				}
				err = productpanel.LaunchReleaseWorkspaceUpdate(directory, revision)
				launchFailed = err != nil
				return err
			})
			s.settingsGate.Unlock()
			if launchFailed {
				// Preserve disk evidence, but require a fresh explicit/periodic
				// verification before retrying a failed handoff.
				s.release.gate.Lock()
				s.release.readyGeneration = 0
				s.release.gate.Unlock()
				s.setRelease("FAILED", generation)
			}
			if applied {
				return
			}
		}
	}
}
