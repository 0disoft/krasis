//go:build windows && desktop

package workspacehost

import (
	"github.com/0disoft/krasis/internal/appsettings"
	"testing"
)

func TestAutoUpdatePreferenceSavesDespiteBrokenRecoveryRegistration(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: false, AutoUpdate: true}); err != nil {
		t.Fatal(err)
	}
	s := &WorkspaceService{}
	s.recoveryFailed.Store(true)
	_ = s.SetAppSettings(false, false)
	v, err := appsettings.Load(base)
	if err != nil || v.AutoUpdate || v.StartAtLogin || !s.recoveryFailed.Load() {
		t.Fatal("unrelated recovery failure blocked update preference or was erased", v, err)
	}
}
