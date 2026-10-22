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
	s.launchRecordFailed.Store(true)
	s.recoveryFailed.Store(true)
	_ = s.SetAppSettings(false, false)
	v, err := appsettings.Load(base)
	if err != nil || v.AutoUpdate || v.StartAtLogin || !s.launchRecordFailed.Load() || !s.recoveryFailed.Load() {
		t.Fatal("unrelated recovery failure blocked update preference or was erased", v, err)
	}
}

func TestPreferenceLeaseFencesGUIRegistrationAndSave(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	initial := appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: true}
	if err := appsettings.Save(base, initial); err != nil {
		t.Fatal(err)
	}
	lease, err := appsettings.LockPreferences(base)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	startup, recovery, writes := true, true, 0
	write := func(value *bool) func(string, bool) error {
		return func(_ string, enabled bool) error {
			writes++
			if err := appsettings.Save(base, initial); err == nil {
				t.Fatal("registration was not protected from another writer")
			}
			*value = enabled
			return nil
		}
	}
	s := &WorkspaceService{startupOps: &startupSettingsOps{
		startupRead: func(string) (bool, error) { return startup, nil }, startupWrite: write(&startup),
		recoveryRead: func(string) (bool, error) { return recovery, nil }, recoveryWrite: write(&recovery),
	}}
	if result := s.SetAppSettings(false, false); result.Code != appsettings.StatusSaveFailed || writes != 0 {
		t.Fatal("GUI mutated registrations while setup held the lease", result, writes)
	}
	if err := lease.Close(); err != nil {
		t.Fatal(err)
	}
	if result := s.SetAppSettings(false, false); result.Code != appsettings.StatusOK || writes != 2 {
		t.Fatal("GUI failed after setup released the lease", result, writes)
	}
	if value, err := appsettings.Load(base); err != nil || value.StartAtLogin || value.AutoUpdate || startup || recovery {
		t.Fatal("saved preferences and registrations diverged", value, err)
	}
}
