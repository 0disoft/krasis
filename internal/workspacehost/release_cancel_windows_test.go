//go:build windows && desktop

package workspacehost

import (
	"context"
	"fmt"
	"testing"

	"github.com/0disoft/krasis/internal/appsettings"
)

func TestReleaseCancellationRequiresUserIntentAndContextCancellation(t *testing.T) {
	for _, tc := range []struct {
		user      bool
		err       error
		state     string
		completed bool
	}{
		{true, fmt.Errorf("wrapped: %w", context.Canceled), "CANCELLED", true},
		{false, context.Canceled, "FAILED", false},
		{true, context.DeadlineExceeded, "FAILED", false},
		{true, fmt.Errorf("download failed"), "FAILED", false},
	} {
		s := &WorkspaceService{}
		s.release.cancelledByUser = tc.user
		if s.finishReleaseError(tc.err, ReleaseSnapshot{}, 900) != tc.completed || s.ReleaseStatus().State != tc.state {
			t.Fatal("failure and user cancellation confused", tc, s.ReleaseStatus())
		}
	}
	s := &WorkspaceService{}
	if s.finishReleaseError(context.DeadlineExceeded, ReleaseSnapshot{State: "READY", Generation: 899}, 900) || s.ReleaseStatus() != (ReleaseSnapshot{State: "READY", Generation: 899}) {
		t.Fatal("failed checking discarded staged update")
	}
}

func TestDisablingUpdatesCancelsQueuedCheckBeforeNetworkWork(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, AutoUpdate: true}); err != nil {
		t.Fatal(err)
	}
	s := &WorkspaceService{ctx: context.Background(), startupOps: &startupSettingsOps{
		startupRead:  func(string) (bool, error) { return false, nil },
		recoveryRead: func(string) (bool, error) { return false, nil },
	}}
	s.release.wake = make(chan struct{}, 1)
	s.CheckRelease()
	if got := s.SetAppSettings(false, false); got.Code != "ok" {
		t.Fatal(got)
	}
	if !s.checkRelease() || s.ReleaseStatus().State != "CANCELLED" {
		t.Fatal("queued cancellation lost", s.ReleaseStatus())
	}
	if got := s.CheckRelease(); got.State != "CHECKING" || s.release.cancelledByUser {
		t.Fatal("new check inherited old cancellation", got)
	}
}

func TestUnrelatedSettingsWriteDoesNotCancelManualCheck(t *testing.T) {
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := &WorkspaceService{startupOps: &startupSettingsOps{startupRead: func(string) (bool, error) { return false, nil }, recoveryRead: func(string) (bool, error) { return false, nil }}}
	s.release.cancel = cancel
	s.release.value.State = "CHECKING"
	s.SetAppSettings(false, false)
	if ctx.Err() != nil || s.release.cancelledByUser {
		t.Fatal("unrelated settings canceled a manual check")
	}
}

func TestUpdateReenableWaitsForCancellationAndCoalescesFollowup(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(fmt.Sprint("active=", active), func(t *testing.T) {
			base := t.TempDir()
			t.Setenv("LOCALAPPDATA", base)
			if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, AutoUpdate: true}); err != nil {
				t.Fatal(err)
			}
			s := &WorkspaceService{ctx: context.Background(), startupOps: &startupSettingsOps{
				startupRead: func(string) (bool, error) { return false, nil }, recoveryRead: func(string) (bool, error) { return false, nil },
			}}
			s.release.wake = make(chan struct{}, 1)
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if active {
				s.release.value.State = "DOWNLOADING"
				s.release.cancel = cancel
			} else {
				s.CheckRelease()
			}
			s.SetAppSettings(false, false)
			s.SetAppSettings(false, true)
			for i := 0; i < 3; i++ {
				s.CheckRelease()
			}
			if !s.release.recheckAfterCancel || !s.release.cancelledByUser || s.ReleaseStatus().State != "CHECKING" {
				t.Fatal("reenable was lost or revived canceled work")
			}
			if active {
				if ctx.Err() != context.Canceled || len(s.release.wake) != 0 {
					t.Fatal("canceled download restarted before completion")
				}
				s.finishReleaseError(ctx.Err(), ReleaseSnapshot{}, 900)
				s.finishReleaseCheck()
			} else if !s.checkRelease() {
				t.Fatal("queued cancellation failed")
			}
			if !s.release.checkPending || s.release.recheckAfterCancel || s.release.cancelledByUser || len(s.release.wake) != 1 {
				t.Fatal("completion did not queue exactly one fresh check")
			}
		})
	}
}

func TestUpdateDisableAgainAndShutdownDiscardDeferredRecheck(t *testing.T) {
	for _, shutdown := range []bool{false, true} {
		base := t.TempDir()
		t.Setenv("LOCALAPPDATA", base)
		if err := appsettings.Save(base, appsettings.Settings{Schema: appsettings.Schema, AutoUpdate: true}); err != nil {
			t.Fatal(err)
		}
		ctx, cancel := context.WithCancel(context.Background())
		s := &WorkspaceService{ctx: ctx, startupOps: &startupSettingsOps{
			startupRead: func(string) (bool, error) { return false, nil }, recoveryRead: func(string) (bool, error) { return false, nil },
		}}
		s.release.wake = make(chan struct{}, 1)
		s.release.value.State = "DOWNLOADING"
		s.release.cancel = func() {}
		s.SetAppSettings(false, false)
		s.SetAppSettings(false, true)
		if shutdown {
			cancel()
		} else {
			s.SetAppSettings(false, false)
		}
		s.finishReleaseCheck()
		cancel()
		if s.release.recheckAfterCancel || s.release.checkPending || len(s.release.wake) != 0 {
			t.Fatal("obsolete follow-up survived cancellation or shutdown")
		}
	}
}
