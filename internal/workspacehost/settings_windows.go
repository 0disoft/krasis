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
		return AppSettingsSnapshot{Schema: appsettings.Schema, Code: "startup_failed"}
	}
	value := appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: startAtLogin, AutoUpdate: autoUpdate}
	if err = appsettings.Save(base, value); err != nil {
		_ = appsettings.SetStartup(base, old.StartAtLogin)
		return AppSettingsSnapshot{Schema: appsettings.Schema, Code: "save_failed", StartAtLogin: old.StartAtLogin, AutoUpdate: old.AutoUpdate}
	}
	return s.appSettings()
}
func (s *WorkspaceService) prepareAppSettings() error {
	base := os.Getenv("LOCALAPPDATA")
	v, err := appsettings.Load(base)
	if err != nil {
		return err
	}
	return appsettings.SetStartup(base, v.StartAtLogin)
}
func (s *WorkspaceService) rememberConnection(v productpanel.BridgeSnapshot) {
	if v.Code != "ok" || (v.State != "CONNECTED" && v.State != "CONNECTING" && v.State != "RECONNECTING") {
		return
	}
	for _, peer := range v.Peers {
		if peer.Alias == v.PeerAlias {
			base := os.Getenv("LOCALAPPDATA")
			want := appsettings.Resume{Alias: peer.Alias, Fingerprint: peer.Fingerprint}
			old, err := appsettings.LoadResume(base)
			if err == nil && old == want {
				return
			}
			_ = appsettings.SaveResume(base, want)
			return
		}
	}
}
func (s *WorkspaceService) resumeAtStartup(ctx context.Context, v productpanel.BridgeSnapshot) productpanel.BridgeSnapshot {
	if !s.startAtLogin || v.State != "LOCAL" || v.Code != "ok" {
		s.rememberConnection(v)
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
