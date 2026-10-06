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
	v, err := appsettings.Load(os.Getenv("LOCALAPPDATA"))
	result := AppSettingsSnapshot{Schema: appsettings.Schema, Code: "unavailable"}
	if err != nil {
		return result
	}
	result.StartAtLogin, result.AutoUpdate = v.StartAtLogin, v.AutoUpdate
	actual, err := appsettings.StartupEnabled(os.Getenv("LOCALAPPDATA"))
	if err != nil || actual != v.StartAtLogin {
		result.Code = "startup_failed"
		return result
	}
	result.Code = "ok"
	if s.startMenuFailed.Load() {
		result.Code = "start_menu_failed"
	}
	if s.recoveryFailed.Load() {
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
	if err = appsettings.SetStartup(base, startAtLogin); err != nil {
		_ = appsettings.SetStartup(base, old.StartAtLogin)
		result := s.appSettings()
		result.Code = "startup_failed"
		return result
	}
	value := appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: startAtLogin, AutoUpdate: autoUpdate}
	if err = appsettings.SetRecoveryTask(base, startAtLogin); err != nil {
		_ = appsettings.SetStartup(base, old.StartAtLogin)
		s.recoveryFailed.Store(true)
		result := s.appSettings()
		result.Code = "recovery_failed"
		return result
	}
	if err = appsettings.Save(base, value); err != nil {
		rollback := appsettings.SetStartup(base, old.StartAtLogin)
		if err := appsettings.SetRecoveryTask(base, old.StartAtLogin); err != nil {
			rollback = err
		}
		result := s.appSettings()
		result.Code = "save_failed"
		if rollback != nil {
			result.Code = "startup_failed"
		}
		return result
	}
	if autoUpdate && !old.AutoUpdate {
		s.CheckRelease()
	}
	if !autoUpdate {
		s.release.gate.Lock()
		if s.release.cancel != nil {
			s.release.cancel()
		}
		s.release.gate.Unlock()
	}
	s.recoveryFailed.Store(false)
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
