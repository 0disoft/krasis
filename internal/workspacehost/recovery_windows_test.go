//go:build windows && desktop

package workspacehost

import (
	"context"
	"errors"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/application"
)

func TestRecoveryRegistrationDoesNotDependOnRendererOrLogger(t *testing.T) {
	for _, failure := range []error{nil, errors.New("registration refused")} {
		calls, menus := 0, 0
		service := WorkspaceService{prepareRecovery: func() error { calls++; return failure }, prepareStartMenu: func() error { menus++; return errors.New("shortcut refused") }}
		if err := service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil || calls != 0 {
			t.Fatal("recovery registration did not run at startup", err)
		}
		service.promoteRecovery()
		wantMenus := 0
		if failure == nil {
			wantMenus = 1
		}
		if menus != wantMenus {
			t.Fatal("shortcut did not follow recovery registration")
		}
		if err := service.ServiceShutdown(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestRecoveryRegistrationRetriesOnlyFailedStages(t *testing.T) {
	for failed := 0; failed < 3; failed++ {
		t.Run([]string{"record", "start_menu", "settings"}[failed], func(t *testing.T) {
			var calls [3]int
			prepare := func(stage int) func() error {
				return func() error {
					calls[stage]++
					if stage == failed && calls[stage] == 1 {
						return errors.New("temporarily unavailable")
					}
					return nil
				}
			}
			s := &WorkspaceService{prepareRecovery: prepare(0), prepareStartMenu: prepare(1), prepareSettings: prepare(2)}
			s.promoteRecovery()
			want := [3]int{1, 1, 1}
			if failed == 0 {
				want = [3]int{1, 0, 0}
			}
			if calls != want || s.launchRecordFailed.Load() != (failed == 0) ||
				s.startMenuFailed.Load() != (failed == 1) || s.recoveryFailed.Load() != (failed == 2) {
				t.Fatal("failed stage was hidden or dependency order changed", calls)
			}
			s.promoteRecovery()
			want = [3]int{1, 1, 1}
			want[failed]++
			if calls != want || !s.recoveryRecordReady || !s.recoveryMenuReady || !s.recoverySettingsReady ||
				s.launchRecordFailed.Load() || s.startMenuFailed.Load() || s.recoveryFailed.Load() {
				t.Fatal("retry repeated successful work or retained an old failure", calls)
			}
			s.promoteRecovery()
			if calls != want {
				t.Fatal("completed registration was repeated", calls)
			}
		})
	}
}

func TestRecoveryRegistrationDoesNotHideIndependentFailures(t *testing.T) {
	menus, settings := 0, 0
	s := &WorkspaceService{
		prepareRecovery: func() error { return nil },
		prepareStartMenu: func() error {
			menus++
			if menus == 1 {
				return errors.New("shortcut unavailable")
			}
			return nil
		},
		prepareSettings: func() error { settings++; return errors.New("task unavailable") },
	}
	s.promoteRecovery()
	if !s.startMenuFailed.Load() || !s.recoveryFailed.Load() || settings != 1 {
		t.Fatal("menu failure prevented independent recovery task registration")
	}
	s.promoteRecovery()
	if s.startMenuFailed.Load() || !s.recoveryFailed.Load() || menus != 2 || settings != 2 {
		t.Fatal("menu repair hid the persistent recovery task failure")
	}
}
