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
		calls := 0
		service := WorkspaceService{prepareRecovery: func() error { calls++; return failure }}
		if err := service.ServiceStartup(context.Background(), application.ServiceOptions{}); err != nil || calls != 1 {
			t.Fatal("recovery registration did not run at startup", err)
		}
		if err := service.ServiceShutdown(); err != nil {
			t.Fatal(err)
		}
	}
}
