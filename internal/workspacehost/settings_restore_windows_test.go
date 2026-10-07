//go:build windows && desktop

package workspacehost

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/0disoft/krasis/internal/appsettings"
)

func TestFailedRecoveryRegistrationRestoresAndVerifiesBothSettings(t *testing.T) {
	for _, mode := range []string{"restored", "query_error", "write_error", "mismatch"} {
		t.Run(mode, func(t *testing.T) {
			base := t.TempDir()
			t.Setenv("LOCALAPPDATA", base)
			old := appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: false, AutoUpdate: false}
			if err := appsettings.Save(base, old); err != nil {
				t.Fatal(err)
			}
			startup, recovery, restoreWrites := false, false, 0
			failure := errors.New("injected registration failure")
			s := &WorkspaceService{startupOps: &startupSettingsOps{
				startupRead:  func(string) (bool, error) { return startup, nil },
				startupWrite: func(_ string, enabled bool) error { startup = enabled; return nil },
				recoveryRead: func(string) (bool, error) {
					if mode == "query_error" {
						return false, failure
					}
					return recovery, nil
				},
				recoveryWrite: func(_ string, enabled bool) error {
					if enabled {
						recovery = true
						return failure
					}
					restoreWrites++
					if mode == "write_error" {
						return failure
					}
					if mode != "mismatch" {
						recovery = false
					}
					return nil
				},
			}}
			got := s.SetAppSettings(true, true)
			want := "rollback_failed"
			if mode == "restored" {
				want = "recovery_failed"
			}
			if got.Code != want || startup || !s.recoveryFailed.Load() {
				t.Fatal("unverified restoration accepted", got, startup)
			}
			if mode == "restored" && recovery {
				t.Fatal("partially changed task survived restoration")
			}
			if mode == "query_error" && restoreWrites != 0 {
				t.Fatal("unknown task state triggered a blind write")
			}
			if mode != "restored" && s.AppSettings().Code != "rollback_failed" {
				t.Fatal("next settings query erased unverified restoration")
			}
			actual, err := appsettings.Load(base)
			if err != nil || actual != old {
				t.Fatal("failed change rewrote saved preferences", actual, err)
			}
		})
	}
}

func TestUnverifiedRestorationClearsOnlyAfterBothStatesAreKnownAndMatched(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema}); err != nil {
		t.Fatal(err)
	}
	startup, recovery, queryFailed := true, true, false
	s := &WorkspaceService{startupOps: &startupSettingsOps{
		startupRead: func(string) (bool, error) {
			if queryFailed {
				return false, errors.New("injected query failure")
			}
			return startup, nil
		},
		recoveryRead: func(string) (bool, error) { return recovery, nil },
	}}
	s.settingsRestorePending.Store(true)
	s.recoveryFailed.Store(true)
	if s.AppSettings().Code != "rollback_failed" {
		t.Fatal("startup mismatch erased restoration state")
	}
	startup = false
	if s.AppSettings().Code != "rollback_failed" {
		t.Fatal("one matched setting accepted as full restoration")
	}
	recovery = false
	queryFailed = true
	if s.AppSettings().Code != "rollback_failed" {
		t.Fatal("unknown read accepted as restoration")
	}
	queryFailed = false
	if got := s.AppSettings(); got.Code != "ok" || s.settingsRestorePending.Load() || s.recoveryFailed.Load() {
		t.Fatal("verified restoration remained a false failure", got)
	}
}

func TestFailedStartupWriteDoesNotTouchRecoveryTask(t *testing.T) {
	startup, recoveryWrites := true, 0
	s := &WorkspaceService{startupOps: &startupSettingsOps{
		startupRead:   func(string) (bool, error) { return startup, nil },
		startupWrite:  func(_ string, enabled bool) error { startup = enabled; return nil },
		recoveryRead:  func(string) (bool, error) { return false, nil },
		recoveryWrite: func(string, bool) error { recoveryWrites++; return nil },
	}}
	if !s.restoreStartupSettings("unused-test-root", false, false) || startup || recoveryWrites != 0 {
		t.Fatal("untouched recovery task was changed")
	}
}

func TestUnreadablePreferencesKeepRestorationFailureWithoutInventingChecks(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: true, AutoUpdate: true}); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(base, "Krasis", "launch", "settings.json")
	if err := os.WriteFile(path, []byte("{}"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &WorkspaceService{}
	s.settingsRestorePending.Store(true)
	if got := s.AppSettings(); got.Code != "rollback_failed" || got.PreferencesKnown {
		t.Fatal("unknown preferences advertised as read", got)
	}
	if got := s.SetAppSettings(false, false); got.Code != "rollback_failed" || got.PreferencesKnown {
		t.Fatal("unreadable settings changed", got)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "{}" {
		t.Fatal("invalid preferences overwritten", err)
	}
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: true, AutoUpdate: true}); err != nil {
		t.Fatal(err)
	}
	s.startupOps = &startupSettingsOps{startupRead: func(string) (bool, error) { return false, nil }, recoveryRead: func(string) (bool, error) { return false, nil }}
	if got := s.AppSettings(); got.Code != "rollback_failed" || !got.PreferencesKnown || !got.StartAtLogin || !got.AutoUpdate {
		t.Fatal("readable preferences conflated with OS restoration", got)
	}
}
