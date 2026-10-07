//go:build windows && desktop

package workspacehost

import (
	"context"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productpanel"
	"os"
)

type AppSettingsSnapshot struct {
	Schema       string `json:"schema"`
	Code         string `json:"code"`
	StartAtLogin bool   `json:"startAtLogin"`
	AutoUpdate   bool   `json:"autoUpdate"`
}

func (s *WorkspaceService) AppSettings() AppSettingsSnapshot {
	s.settingsGate.Lock()
	defer s.settingsGate.Unlock()
	return s.appSettings()
}
func (s *WorkspaceService) appSettings() AppSettingsSnapshot {
	restorePending := s.settingsRestorePending.Load()
	v, err := appsettings.Load(os.Getenv("LOCALAPPDATA"))
	result := AppSettingsSnapshot{Schema: appsettings.Schema, Code: "unavailable"}
	if err != nil {
		if restorePending {
			result.Code = "rollback_failed"
		}
		return result
	}
	result.StartAtLogin, result.AutoUpdate = v.StartAtLogin, v.AutoUpdate
	ops := s.startupSettings()
	actual, err := ops.startupRead(os.Getenv("LOCALAPPDATA"))
	if err != nil || actual != v.StartAtLogin {
		result.Code = "startup_failed"
		if restorePending {
			result.Code = "rollback_failed"
		}
		return result
	}
	active, err := ops.recoveryRead(os.Getenv("LOCALAPPDATA"))
	if err != nil || active != v.StartAtLogin {
		result.Code = "recovery_failed"
		if restorePending {
			result.Code = "rollback_failed"
		}
		return result
	}
	if restorePending {
		s.settingsRestorePending.Store(false)
		s.recoveryFailed.Store(false)
		s.recoverySettingsReady = true
		s.log.Event("settings_restore_all_verified")
	}
	result.Code = "ok"
	if s.startMenuFailed.Load() {
		result.Code = "start_menu_failed"
	}
	if s.launchRecordFailed.Load() || s.recoveryFailed.Load() {
		result.Code = "recovery_failed"
	}
	return result
}
func (s *WorkspaceService) SetAppSettings(startAtLogin, autoUpdate bool) AppSettingsSnapshot {
	s.settingsGate.Lock()
	defer s.settingsGate.Unlock()
	base := os.Getenv("LOCALAPPDATA")
	old, err := appsettings.Load(base)
	if err != nil {
		return s.appSettings()
	}
	startupChanged := startAtLogin != old.StartAtLogin
	ops := s.startupSettings()
	if startupChanged {
		if err = ops.startupWrite(base, startAtLogin); err != nil {
			s.log.Event("settings_change_startup_failed")
			restored := s.restoreStartupSettings(base, old.StartAtLogin, false)
			result := s.appSettings()
			result.Code = "startup_failed"
			if !restored {
				result.Code = "rollback_failed"
			}
			return result
		}
		if err = ops.recoveryWrite(base, startAtLogin); err != nil {
			s.log.Event("settings_change_recovery_failed")
			restored := s.restoreStartupSettings(base, old.StartAtLogin, true)
			s.recoveryFailed.Store(true)
			s.recoverySettingsReady = false
			result := s.appSettings()
			result.Code = "recovery_failed"
			if !restored {
				result.Code = "rollback_failed"
			}
			return result
		}
	}
	value := appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: startAtLogin, AutoUpdate: autoUpdate}
	if err = appsettings.Save(base, value); err != nil {
		s.log.Event("settings_change_save_failed")
		restored := true
		if startupChanged {
			restored = s.restoreStartupSettings(base, old.StartAtLogin, true)
		}
		result := s.appSettings()
		result.Code = "save_failed"
		if !restored {
			result.Code = "rollback_failed"
		}
		return result
	}
	if autoUpdate && !old.AutoUpdate {
		s.CheckRelease()
	}
	if !autoUpdate && old.AutoUpdate {
		s.release.gate.Lock()
		s.release.recheckAfterCancel = false
		if s.release.cancel != nil || s.release.checkPending || s.release.value.State == "CHECKING" || s.release.value.State == "DOWNLOADING" {
			s.release.cancelledByUser = true
		}
		if s.release.cancel != nil {
			s.release.cancel()
		}
		s.release.gate.Unlock()
	}
	if startupChanged && (s.prepareSettings == nil || s.recoverySettingsReady) {
		s.recoveryFailed.Store(false)
	}
	return s.appSettings()
}
func (s *WorkspaceService) prepareAppSettings() error {
	base := os.Getenv("LOCALAPPDATA")
	v, err := appsettings.Load(base)
	if err != nil {
		return err
	}
	if err = appsettings.SetStartup(base, v.StartAtLogin); err != nil {
		return err
	}
	return appsettings.SetRecoveryTask(base, v.StartAtLogin)
}
func (s *WorkspaceService) resumeAtStartup(ctx context.Context, v productpanel.BridgeSnapshot) productpanel.BridgeSnapshot {
	if !s.startAtLogin || v.State != "LOCAL" || v.Code != "ok" {
		return v
	}
	peer, err := appsettings.LoadResume(os.Getenv("LOCALAPPDATA"))
	if err != nil || peer.Alias == "" {
		return v
	}
	for _, p := range v.Peers {
		if p.Alias == peer.Alias && p.Fingerprint == peer.Fingerprint {
			return productpanel.NewDesktopBridge(ctx, func() (productpanel.Client, error) { return localapi.OpenClient(s.control) }).Connect(peer.Alias, peer.Fingerprint)
		}
	}
	return v
}
