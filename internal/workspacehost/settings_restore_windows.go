//go:build windows && desktop

package workspacehost

import "github.com/0disoft/krasis/internal/appsettings"

// Per-service operations let failure tests avoid the user's registry and tasks.
type startupSettingsOps struct {
	startupRead   func(string) (bool, error)
	startupWrite  func(string, bool) error
	recoveryRead  func(string) (bool, error)
	recoveryWrite func(string, bool) error
}

func (s *WorkspaceService) startupSettings() startupSettingsOps {
	if s.startupOps != nil {
		return *s.startupOps
	}
	return startupSettingsOps{appsettings.StartupEnabled, appsettings.SetStartup, appsettings.RecoveryTaskEnabled, appsettings.SetRecoveryTask}
}

func (s *WorkspaceService) restoreStartupSettings(base string, expected, recoveryTouched bool) bool {
	ops := s.startupSettings()
	restore := func(name string, read func(string) (bool, error), write func(string, bool) error) bool {
		actual, err := read(base)
		if err != nil {
			s.log.Event("settings_restore_" + name + "_query_failed")
			return false
		}
		if actual != expected {
			writeErr := write(base, expected)
			actual, err = read(base)
			if writeErr != nil {
				s.log.Event("settings_restore_" + name + "_write_failed")
				return false
			}
			if err != nil {
				s.log.Event("settings_restore_" + name + "_query_failed")
				return false
			}
		}
		if actual != expected {
			s.log.Event("settings_restore_" + name + "_mismatch")
			return false
		}
		s.log.Event("settings_restore_" + name + "_verified")
		return true
	}
	startupOK := restore("startup", ops.startupRead, ops.startupWrite)
	recoveryOK := true
	if recoveryTouched {
		recoveryOK = restore("recovery", ops.recoveryRead, ops.recoveryWrite)
	}
	if !startupOK || !recoveryOK {
		s.settingsRestorePending.Store(true)
		s.recoveryFailed.Store(true)
		s.recoverySettingsReady = false
	}
	return startupOK && recoveryOK
}
