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
	Schema           string `json:"schema"`
	Code             string `json:"code"`
	StartAtLogin     bool   `json:"startAtLogin"`
	AutoUpdate       bool   `json:"autoUpdate"`
	PreferencesKnown bool   `json:"preferencesKnown"`
}

func (s *WorkspaceService) AppSettings() AppSettingsSnapshot {
	s.settingsGate.Lock()
	defer s.settingsGate.Unlock()
	return s.appSettings()
}
func (s *WorkspaceService) appSettings() AppSettingsSnapshot {
	restorePending := s.settingsRestorePending.Load()
	v, err := appsettings.Load(os.Getenv("LOCALAPPDATA"))
	result := AppSettingsSnapshot{Schema: appsettings.Schema, Code: appsettings.StatusUnavailable}
	if err != nil {
		if restorePending {
			result.Code = appsettings.StatusRollbackFailed
		}
		return result
	}
	result.StartAtLogin, result.AutoUpdate = v.StartAtLogin, v.AutoUpdate
	result.PreferencesKnown = true
	ops := s.startupSettings()
	actual, err := ops.startupRead(os.Getenv("LOCALAPPDATA"))
	if err != nil || actual != v.StartAtLogin {
		result.Code = appsettings.StatusStartupFailed
		if err != nil {
			result.Code = appsettings.StatusStartupQueryFailed
		}
		if restorePending {
			result.Code = appsettings.StatusRollbackFailed
		}
		return result
	}
	active, err := ops.recoveryRead(os.Getenv("LOCALAPPDATA"))
	if err != nil || active != v.StartAtLogin {
		result.Code = appsettings.StatusRecoveryFailed
		if err != nil {
			result.Code = appsettings.StatusRecoveryQueryFailed
		}
		if restorePending {
			result.Code = appsettings.StatusRollbackFailed
		}
		return result
	}
	if restorePending {
		s.settingsRestorePending.Store(false)
		s.recoveryFailed.Store(false)
		s.recoverySettingsReady = true
		s.log.Event("settings_restore_all_verified")
	}
	result.Code = appsettings.StatusOK
	if s.startMenuFailed.Load() {
		result.Code = appsettings.StatusStartMenuFailed
	}
	if s.launchRecordFailed.Load() || s.recoveryFailed.Load() {
		result.Code = appsettings.StatusRecoveryFailed
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
			result.Code = appsettings.StatusStartupFailed
			if !restored {
				result.Code = appsettings.StatusRollbackFailed
			}
			return result
		}
		if err = ops.recoveryWrite(base, startAtLogin); err != nil {
			s.log.Event("settings_change_recovery_failed")
			restored := s.restoreStartupSettings(base, old.StartAtLogin, true)
			s.recoveryFailed.Store(true)
			s.recoverySettingsReady = false
			result := s.appSettings()
			result.Code = appsettings.StatusRecoveryFailed
			if !restored {
				result.Code = appsettings.StatusRollbackFailed
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
		result.Code = appsettings.StatusSaveFailed
		if !restored {
			result.Code = appsettings.StatusRollbackFailed
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
	v, err := appsettings.Initialize(base)
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
	return s.resumeSavedConnection(ctx, v)
}

func (s *WorkspaceService) resumeSavedConnection(ctx context.Context, v productpanel.BridgeSnapshot) productpanel.BridgeSnapshot {
	if v.State != "LOCAL" || v.Code != "ok" {
		return v
	}
	v = productpanel.ResumeAtLogin(ctx, v,
		func() (appsettings.Resume, error) { return appsettings.LoadResume(os.Getenv("LOCALAPPDATA")) },
		func(alias, fingerprint string) productpanel.BridgeSnapshot {
			return productpanel.NewDesktopBridge(ctx, func() (productpanel.Client, error) { return localapi.OpenClient(s.control) }).Resume(alias, fingerprint)
		})
	if v.Code == "startup_resume_settings_invalid" || v.Code == "startup_resume_settings_unavailable" {
		s.log.Event(v.Code)
	}
	return v
}
