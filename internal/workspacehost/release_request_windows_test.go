//go:build windows && desktop

package workspacehost

import "testing"

func TestManualReleaseCheckAcknowledgesAndCoalesces(t *testing.T) {
	s := &WorkspaceService{}
	s.release.wake = make(chan struct{}, 1)
	s.release.value = ReleaseSnapshot{State: "READY", Generation: 900}
	for i := 0; i < 3; i++ {
		if got := s.CheckRelease(); got.State != "CHECKING" {
			t.Fatal("request not acknowledged", got)
		}
	}
	if len(s.release.wake) != 1 || !s.release.checkPending {
		t.Fatal("manual requests were duplicated or lost")
	}
	if got := s.ReleaseStatus(); got.State != "CHECKING" {
		t.Fatal("status exposed the old result", got)
	}
	if s.release.value != (ReleaseSnapshot{State: "READY", Generation: 900}) {
		t.Fatal("pending request discarded the staged release")
	}
	<-s.release.wake
	s.release.checkPending = false
	for _, state := range []string{"CHECKING", "DOWNLOADING"} {
		s.release.value.State = state
		if got := s.CheckRelease(); got.State != state || len(s.release.wake) != 0 {
			t.Fatal("active work queued another check", got)
		}
	}
}

func TestManualReleaseCheckWithoutWorkerDoesNotClaimAccepted(t *testing.T) {
	s := &WorkspaceService{}
	if got := s.CheckRelease(); got.State != "FAILED" || s.release.checkPending {
		t.Fatal("missing worker advertised pending work", got)
	}
}
