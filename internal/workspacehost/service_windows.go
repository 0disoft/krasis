//go:build windows && desktop

package workspacehost

import (
	"context"
	"github.com/0disoft/krasis/internal/launcherui"
	"github.com/0disoft/krasis/internal/launchrecord"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productlog"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/wailsapp/wails/v3/pkg/application"
	"sync"
	"sync/atomic"
	"time"
)

type WorkspaceService struct {
	uiReplacement uiReplacement
	visibleWindow atomic.Pointer[application.WebviewWindow]
	uiReadyEpoch  atomic.Uint64
	release       releaseState
	settingsGate  sync.Mutex
	startAtLogin  bool

	startMenuFailed       atomic.Bool
	launchRecordFailed    atomic.Bool
	recoveryFailed        atomic.Bool
	recoveryRecordReady   bool
	recoveryMenuReady     bool
	recoverySettingsReady bool

	ctx              context.Context
	control          string
	title            string
	clipGate         sync.Mutex
	clipEpoch        atomic.Uint64
	raw              *productpanel.DesktopBridge
	managed          *productpanel.DesktopLifecycle
	pairs            *productpanel.PairingBridge
	diagnostics      *productpanel.DiagnosticsBridge
	updates          *productpanel.UpdateSettingsBridge
	receiver         *productpanel.WorkspaceUpdateReceiver
	quit             func()
	done             chan struct{}
	jobs             sync.WaitGroup
	prepareRecovery  func() error
	prepareStartMenu func() error
	prepareSettings  func() error
	inputRefresh     func(context.Context) error
	inputSetup       func() string
	log              *productlog.Logger
}

func (s *WorkspaceService) promoteRecovery() {
	s.settingsGate.Lock()
	defer s.settingsGate.Unlock()
	if !s.recoveryRecordReady && s.prepareRecovery != nil {
		if err := s.prepareRecovery(); err != nil {
			s.launchRecordFailed.Store(true)
			s.log.Event("launch_record_" + launchrecord.RegistrationStage(err) + "_failed")
			return
		}
		s.launchRecordFailed.Store(false)
		s.recoveryRecordReady = true
		s.log.Event("launch_record_ready")
	}
	if !s.recoveryRecordReady {
		return
	}
	// Keep successful stages; retry failures only on an existing startup/recovery
	// attempt, never from renderer polling or an additional recovery loop.
	if !s.recoveryMenuReady && s.prepareStartMenu != nil {
		err := s.prepareStartMenu()
		s.startMenuFailed.Store(err != nil)
		s.recoveryMenuReady = err == nil
		if err != nil {
			s.log.Event("start_menu_registration_failed")
		}
	}
	if !s.recoverySettingsReady && s.prepareSettings != nil {
		err := s.prepareSettings()
		s.recoveryFailed.Store(err != nil)
		s.recoverySettingsReady = err == nil
		if err != nil {
			s.log.Event("app_settings_startup_failed")
		}
	}
}

func (s *WorkspaceService) ServiceStartup(context.Context, application.ServiceOptions) error {
	if s.managed != nil {
		s.managed.StartupReady = s.promoteRecovery
		s.jobs.Add(1)
		go func() { defer s.jobs.Done(); s.runUIReadyReporter() }()
		s.done = make(chan struct{})
		go func() { defer close(s.done); s.managed.Run() }()
		s.jobs.Add(1)
		go func() {
			defer s.jobs.Done()
			launcherui.RunClipboardWorker(s.ctx, s.control, s.title, &s.clipGate, &s.clipEpoch)
		}()
		if s.receiver != nil && s.quit != nil {
			s.jobs.Add(1)
			go s.runUpdateReceiver()
		}
		if s.quit != nil {
			s.jobs.Add(1)
			go func() {
				handoff := false
				defer func() {
					s.jobs.Done()
					if handoff {
						s.quit()
					}
				}()
				handoff = s.runUIReplacement()
			}()
			s.release.wake = make(chan struct{}, 1)
			s.release.value = ReleaseSnapshot{State: "IDLE"}
			s.jobs.Add(1)
			go s.runReleaseUpdater()
		}
	}
	return nil
}
func (s *WorkspaceService) ServiceShutdown() error {
	if s.done != nil {
		<-s.done
	}
	s.jobs.Wait()
	s.log.Close()
	return nil
}
func (s *WorkspaceService) disconnectAsync() {
	s.jobs.Add(1)
	go func() { defer s.jobs.Done(); s.Disconnect() }()
}
func (s *WorkspaceService) Refresh() productpanel.BridgeSnapshot {
	if s.managed != nil {
		return s.withInputSetup(s.managed.Refresh())
	}
	return s.withInputSetup(s.raw.Refresh())
}

func (s *WorkspaceService) withInputSetup(value productpanel.BridgeSnapshot) productpanel.BridgeSnapshot {
	if s.inputSetup != nil {
		value.InputSetup = s.inputSetup()
	}
	return value
}

func (s *WorkspaceService) InputRefresh() productpanel.BridgeSnapshot {
	if s.managed == nil || s.inputRefresh == nil {
		return s.Refresh()
	}
	return s.withInputSetup(s.managed.RefreshProtectedInput(s.inputRefresh))
}
func (s *WorkspaceService) Connect(alias, fingerprint string) productpanel.BridgeSnapshot {
	if s.managed != nil {
		value := s.withInputSetup(s.managed.Connect(alias, fingerprint))
		return value
	}
	value := s.withInputSetup(s.raw.Connect(alias, fingerprint))
	return value
}
func (s *WorkspaceService) Disconnect() productpanel.BridgeSnapshot {
	if s.managed != nil {
		return s.withInputSetup(s.managed.Disconnect())
	}
	return s.withInputSetup(s.raw.Disconnect())
}
func (s *WorkspaceService) Restart() productpanel.BridgeSnapshot {
	if s.managed != nil {
		return s.withInputSetup(s.managed.Restart())
	}
	return s.withInputSetup(s.raw.Refresh())
}

// Export locally before replacement when available. The lifecycle bounds this
// best-effort task independently of mandatory input/process ownership recovery.
func diagnose(local, control string) func(context.Context, string) error {
	return func(parent context.Context, fingerprint string) error {
		if _, err := productlog.Export(local); err != nil {
			return err
		}
		if err := parent.Err(); err != nil {
			return err
		}
		if len(fingerprint) != 64 {
			return nil
		}
		ctx, cancel := context.WithTimeout(parent, 5*time.Second)
		defer cancel()
		client, err := localapi.OpenClient(control)
		if err != nil {
			return nil
		}
		result, err := client.SendDiagnostic(ctx, fingerprint)
		if err != nil || result.Code != "queued" {
			return nil
		}
		timer := time.NewTicker(250 * time.Millisecond)
		defer timer.Stop()
		for {
			select {
			case <-ctx.Done():
				return nil
			case <-timer.C:
				status, err := client.Update(ctx, localapi.UpdateRequest{Schema: localapi.UpdateSchema, Action: "STATUS"})
				if err != nil || status.Status.Diagnostic != "QUEUED" {
					return nil
				}
			}
		}
	}
}
