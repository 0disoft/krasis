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

func TestManualRefreshRequestsFreshHealthWithoutInstallingOrRestarting(t *testing.T) {
	managed := productpanel.NewDesktopLifecycle(context.Background(), nil, func(context.Context) error { t.Fatal("refresh restarted agent"); return nil }, nil)
	checks := 0
	s := &WorkspaceService{managed: managed, inputSetup: func() string { return "UNKNOWN" },
		inputRecheck: func() string { checks++; return "READY" },
		inputRefresh: func(context.Context) error { t.Fatal("refresh installed input"); return nil }}
	if v := s.Refresh(); v.InputSetup != "UNKNOWN" || checks != 0 {
		t.Fatal(v, checks)
	}
	if v := s.RefreshNow(); v.InputSetup != "READY" || v.State != "UNKNOWN" || checks != 1 {
		t.Fatal(v, checks)
	}
}
