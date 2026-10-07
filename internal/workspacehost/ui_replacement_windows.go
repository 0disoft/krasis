//go:build windows && desktop

package workspacehost

import (
	"bytes"
	"errors"
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
	source              string
	state               string
}

type UIReplacementSnapshot struct {
	Schema     string `json:"schema"`
	State      string `json:"state"`
	Generation int    `json:"generation"`
	CanRetry   bool   `json:"canRetry"`
}

func (s *WorkspaceService) UIReplacementStatus() UIReplacementSnapshot {
	s.uiReplacement.gate.Lock()
	defer s.uiReplacement.gate.Unlock()
	return s.uiReplacementStatus()
}

func (s *WorkspaceService) uiReplacementStatus() UIReplacementSnapshot {
	state := s.uiReplacement.state
	if state == "" {
		state = "IDLE"
	}
	return UIReplacementSnapshot{Schema: "krasis.ui-replacement/v1", State: state, Generation: s.uiReplacement.generation,
		CanRetry: state == "FAILED" && s.uiReplacement.source != "" && s.ctx.Err() == nil}
}

// Retry uses only the last locally opened image, never renderer-supplied paths.
// The source bundle is verified again before it can become an apply candidate.
func (s *WorkspaceService) RetryUIReplacement() UIReplacementSnapshot {
	s.uiReplacement.gate.Lock()
	if s.uiReplacement.state != "FAILED" || s.uiReplacement.source == "" || s.ctx.Err() != nil {
		value := s.uiReplacementStatus()
		s.uiReplacement.gate.Unlock()
		return value
	}
	s.uiReplacement.state = "PREPARING"
	source, revision, generation := s.uiReplacement.source, s.uiReplacement.revision, s.uiReplacement.generation
	value := s.uiReplacementStatus()
	s.jobs.Add(1)
	s.uiReplacement.gate.Unlock()
	go func() { defer s.jobs.Done(); s.stageUIReplacement(source, revision, generation) }()
	return value
}

// Only a complete newer product bundle is staged. IPC arguments never become
// executable arguments; the existing verified updater owns the actual launch.
func (s *WorkspaceService) requestUIReplacement(args []string, data map[string]string) bool {
	generation, err := strconv.Atoi(data["generation"])
	if err != nil || generation <= localapi.AgentGeneration || generation > 1000000 || len(args) == 0 || !filepath.IsAbs(args[0]) || filepath.Base(args[0]) != "krasis.exe" {
		return false
	}
	s.uiReplacement.gate.Lock()
	if s.ctx.Err() != nil || (s.uiReplacement.state != "" && s.uiReplacement.state != "IDLE" && s.uiReplacement.state != "FAILED") {
		s.uiReplacement.gate.Unlock()
		return true
	}
	s.uiReplacement.source, s.uiReplacement.revision, s.uiReplacement.generation = args[0], data["revision"], generation
	s.uiReplacement.directory, s.uiReplacement.state = "", "PREPARING"
	s.uiReplacement.gate.Unlock()
	s.stageUIReplacement(args[0], data["revision"], generation)
	return true
}

func (s *WorkspaceService) stageUIReplacement(source, revision string, generation int) {
	fail := func(code string) {
		s.log.Event(code)
		s.uiReplacement.gate.Lock()
		s.uiReplacement.directory, s.uiReplacement.state = "", "FAILED"
		s.uiReplacement.gate.Unlock()
	}
	if s.ctx.Err() != nil {
		fail("ui_replacement_stage_failed")
		return
	}
	guard, err := securefs.OpenReadDirectory(filepath.Dir(source))
	if err != nil {
		fail("ui_replacement_bundle_failed")
		return
	}
	defer guard.Close()
	archive, err := updatebundle.Pack(guard)
	if err != nil {
		fail("ui_replacement_bundle_failed")
		return
	}
	bundle, err := updatebundle.Read(bytes.NewReader(archive), int64(len(archive)))
	if err != nil || bundle.Metadata().Generation != generation || bundle.Metadata().Revision != revision {
		fail("ui_replacement_bundle_failed")
		return
	}
	if s.ctx.Err() != nil {
		fail("ui_replacement_stage_failed")
		return
	}
	directory, err := releaseupdate.Stage(os.Getenv("LOCALAPPDATA"), bundle)
	if err != nil {
		fail("ui_replacement_stage_failed")
		return
	}
	s.uiReplacement.gate.Lock()
	s.uiReplacement.directory, s.uiReplacement.revision, s.uiReplacement.generation = directory, bundle.Metadata().Revision, generation
	s.uiReplacement.state = "PENDING"
	s.uiReplacement.gate.Unlock()
	s.log.Event("ui_replacement_requested")
	if window := s.visibleWindow.Load(); window != nil {
		window.SetTitle("Krasis · g" + strconv.Itoa(localapi.AgentGeneration) + " · 새 버전 대기")
	}
}

func (s *WorkspaceService) finishUIReplacement(handedOff bool, err error) {
	s.uiReplacement.gate.Lock()
	defer s.uiReplacement.gate.Unlock()
	if handedOff {
		s.uiReplacement.state = "APPLIED"
		return
	}
	if err == nil || errors.Is(err, productpanel.ErrStartupBusy) || errors.Is(err, productpanel.ErrAgentBusy) {
		s.uiReplacement.state = "PENDING"
		return
	}
	s.log.Event("ui_replacement_launch_failed")
	s.uiReplacement.directory, s.uiReplacement.state = "", "FAILED"
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
			if directory == "" || s.uiReplacement.state != "PENDING" {
				s.uiReplacement.gate.Unlock()
				continue
			}
			s.uiReplacement.state = "APPLYING"
			s.uiReplacement.gate.Unlock()
			handedOff, err := s.managed.ApplyRequestedUpdate(generation, revision, func(alias, fingerprint string) error {
				return productpanel.LaunchRequestedWorkspaceUpdate(directory, revision, alias, fingerprint)
			})
			s.finishUIReplacement(handedOff, err)
			if handedOff {
				return true
			}
			if err != nil && !errors.Is(err, productpanel.ErrStartupBusy) && !errors.Is(err, productpanel.ErrAgentBusy) {
				if window := s.visibleWindow.Load(); window != nil {
					window.SetTitle(s.title)
				}
			}
		}
	}
}
