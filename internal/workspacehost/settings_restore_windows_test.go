//go:build windows && desktop

package workspacehost

import (
	"errors"
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
			actual, err := appsettings.Load(base)
			if err != nil || actual != old {
				t.Fatal("failed change rewrote saved preferences", actual, err)
			}
		})
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
