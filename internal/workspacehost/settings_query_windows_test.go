//go:build windows && desktop

package workspacehost

import (
	"errors"
	"testing"

	"github.com/0disoft/krasis/internal/appsettings"
)

func TestRegistrationQueryFailuresDoNotClaimFailedRegistration(t *testing.T) {
	for _, stage := range []string{"startup", "recovery"} {
		for _, mode := range []string{"query_error", "mismatch", "rollback_query_error", "verified"} {
			t.Run(stage+"/"+mode, func(t *testing.T) {
				base := t.TempDir()
				t.Setenv("LOCALAPPDATA", base)
				if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, StartAtLogin: true, AutoUpdate: false}); err != nil {
					t.Fatal(err)
				}
				read := func(name string) func(string) (bool, error) {
					return func(string) (bool, error) {
						if name == stage {
							switch mode {
							case "query_error", "rollback_query_error":
								return false, errors.New("private scheduler query detail")
							case "mismatch":
								return false, nil
							}
						}
						return true, nil
					}
				}
				writes := 0
				write := func(string, bool) error { writes++; return nil }
				s := &WorkspaceService{startupOps: &startupSettingsOps{startupRead: read("startup"), recoveryRead: read("recovery"), startupWrite: write, recoveryWrite: write}}
				s.settingsRestorePending.Store(mode == "rollback_query_error")
				want := "ok"
				switch mode {
				case "query_error":
					want = stage + "_query_failed"
				case "mismatch":
					want = stage + "_failed"
				case "rollback_query_error":
					want = "rollback_failed"
				}
				got := s.AppSettings()
				if got.Code != want || !got.PreferencesKnown || !got.StartAtLogin || got.AutoUpdate || writes != 0 {
					t.Fatal("query uncertainty, confirmed mismatch or saved preferences changed", got, "writes", writes)
				}
			})
		}
	}
}
