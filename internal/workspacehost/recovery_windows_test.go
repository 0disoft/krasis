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
