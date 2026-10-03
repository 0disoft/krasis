//go:build windows && desktop

package workspacehost

import (
	"context"
	"testing"

	"github.com/0disoft/krasis/internal/productpanel"
)

func TestWorkspacePublishesInstallationProgressWithoutChangingAgentState(t *testing.T) {
	managed := productpanel.NewDesktopLifecycle(context.Background(), nil, nil, nil)
	setup := "UPDATING"
	s := &WorkspaceService{managed: managed, inputSetup: func() string { return setup }}
	value := s.Refresh()
	if value.InputSetup != "UPDATING" || value.State != "UNKNOWN" || value.InputBackend != "UNKNOWN" {
		t.Fatal(value)
	}
	setup = "FAILED"
	value = s.Refresh()
	if value.InputSetup != "FAILED" || value.State != "UNKNOWN" {
		t.Fatal(value)
	}
}
