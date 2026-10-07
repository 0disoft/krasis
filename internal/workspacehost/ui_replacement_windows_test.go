//go:build windows && desktop

package workspacehost

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productpanel"
)

func TestUIReplacementFailureIsVisibleAndRetryRevalidatesOriginalSource(t *testing.T) {
	s := &WorkspaceService{ctx: context.Background()}
	source := filepath.Join(t.TempDir(), "missing", "krasis.exe")
	generation := localapi.AgentGeneration + 1
	if !s.requestUIReplacement([]string{source}, map[string]string{"generation": strconv.Itoa(generation), "revision": strings.Repeat("a", 40)}) {
		t.Fatal("new version not handled")
	}
	if got := s.UIReplacementStatus(); got.State != "FAILED" || got.Generation != generation || !got.CanRetry {
		t.Fatal(got)
	}
	if got := s.RetryUIReplacement(); got.State != "PREPARING" || got.CanRetry {
		t.Fatal(got)
	}
	s.jobs.Wait()
	if got := s.UIReplacementStatus(); got.State != "FAILED" || !got.CanRetry || s.uiReplacement.directory != "" {
		t.Fatal("missing source bypassed retry verification", got)
	}
}

func TestUIReplacementCoalescesWorkAndSeparatesBusyFromFailure(t *testing.T) {
	for _, tc := range []struct {
		state string
		err   error
	}{{"PENDING", productpanel.ErrStartupBusy}, {"PENDING", productpanel.ErrAgentBusy}, {"FAILED", errors.New("private launch error")}} {
		s := &WorkspaceService{ctx: context.Background()}
		s.uiReplacement.source, s.uiReplacement.directory, s.uiReplacement.generation, s.uiReplacement.state = "original", "verified", localapi.AgentGeneration+1, "APPLYING"
		if got := s.RetryUIReplacement(); got.State != "APPLYING" || got.CanRetry {
			t.Fatal("active replacement retried", got)
		}
		s.finishUIReplacement(false, tc.err)
		if got := s.UIReplacementStatus(); got.State != tc.state || got.CanRetry != (tc.state == "FAILED") {
			t.Fatal(got)
		}
		if tc.state == "FAILED" && s.uiReplacement.directory != "" {
			t.Fatal("failed candidate still applies automatically")
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	s := &WorkspaceService{ctx: ctx}
	s.uiReplacement.source, s.uiReplacement.state, s.uiReplacement.generation = "original", "FAILED", localapi.AgentGeneration+1
	if got := s.RetryUIReplacement(); got.CanRetry || got.State != "FAILED" {
		t.Fatal("retry admitted after shutdown", got)
	}
}
