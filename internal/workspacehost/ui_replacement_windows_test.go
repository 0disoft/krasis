//go:build windows && desktop

package workspacehost

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productidentity"
	"github.com/0disoft/krasis/internal/productpanel"
	"github.com/0disoft/krasis/internal/updatebundle"
)

func TestUIReplacementFailureIsVisibleAndRetryRevalidatesOriginalSource(t *testing.T) {
	s := &WorkspaceService{ctx: context.Background()}
	source := filepath.Join(t.TempDir(), "missing", "krasis.exe")
	generation := localapi.AgentGeneration + 1
	if !s.requestUIReplacement([]string{source}, map[string]string{"generation": strconv.Itoa(generation), "revision": strings.Repeat("a", 40)}) {
		t.Fatal("new version not handled")
	}
	s.jobs.Wait()
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

func TestUIReplacementRequestReturnsWhilePreparationIsStillBlocked(t *testing.T) {
	s := &WorkspaceService{ctx: context.Background()}
	started, unblock := make(chan struct{}), make(chan struct{})
	s.uiReplacement.prepare = func(string, string, int) { close(started); <-unblock }
	defer func() { close(unblock); s.jobs.Wait() }()
	source := filepath.Join(t.TempDir(), "krasis.exe")
	data := map[string]string{"generation": strconv.Itoa(localapi.AgentGeneration + 1), "revision": strings.Repeat("a", 40)}
	returned := make(chan bool, 1)
	go func() { returned <- s.requestUIReplacement([]string{source}, data) }()
	select {
	case got := <-returned:
		if !got {
			t.Fatal("request not admitted")
		}
	case <-time.After(time.Second):
		t.Fatal("window callback waits for file preparation")
	}
	<-started
	if got := s.UIReplacementStatus(); got.State != "PREPARING" || got.CanRetry {
		t.Fatal("preparation status blocked or retriable", got)
	}
	if !s.requestUIReplacement([]string{source}, data) {
		t.Fatal("duplicate request not coalesced")
	}
	if got := s.RetryUIReplacement(); got.State != "PREPARING" {
		t.Fatal("retry duplicated blocked preparation", got)
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
		if tc.state == "FAILED" && s.uiReplacement.directory != "verified" {
			t.Fatal("failed launch discarded reusable stage")
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

// Opt-in: validate real packaged images without executing any of them.
func TestUIReplacementRetryReusesVerifiedPackageAndRejectsChangedStage(t *testing.T) {
	fixture := os.Getenv("KRASIS_REPLACEMENT_SMOKE")
	if fixture == "" {
		t.Skip("requires real package fixture")
	}
	data, err := os.ReadFile(filepath.Join(fixture, "BUILD.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta updatebundle.Metadata
	if err = json.Unmarshal(data, &meta); err != nil {
		t.Fatal(err)
	}
	sourceDir := t.TempDir()
	files, err := os.ReadDir(fixture)
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if file.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(fixture, file.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(filepath.Join(sourceDir, file.Name()), data, 0600); err != nil {
			t.Fatal(err)
		}
	}
	base := t.TempDir()
	t.Setenv("LOCALAPPDATA", base)
	root, err := productidentity.PrepareLaunch(base)
	if err != nil {
		t.Fatal(err)
	}
	root.Close()
	s := &WorkspaceService{ctx: context.Background()}
	source := filepath.Join(sourceDir, "krasis.exe")
	s.uiReplacement.source, s.uiReplacement.revision, s.uiReplacement.generation = source, meta.Revision, meta.Generation
	s.stageUIReplacement(source, meta.Revision, meta.Generation)
	stage := s.uiReplacement.directory
	if stage == "" || s.UIReplacementStatus().State != "PENDING" {
		t.Fatal("package did not stage")
	}
	s.finishUIReplacement(false, errors.New("launch failed"))
	// Destroy only the isolated source copy; retained stage must stand alone.
	if err = os.WriteFile(source, []byte("changed source"), 0600); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if got := s.RetryUIReplacement(); got.State != "PREPARING" {
			t.Fatal(got)
		}
		s.jobs.Wait()
		if s.uiReplacement.directory != stage || s.UIReplacementStatus().State != "PENDING" {
			t.Fatal("retry replaced the verified stage")
		}
		s.finishUIReplacement(false, errors.New("launch failed"))
	}
	entries, err := os.ReadDir(filepath.Join(base, "Krasis", "updates"))
	if err != nil || len(entries) != 1 {
		t.Fatal("retry created redundant stages", len(entries), err)
	}
	if err = os.WriteFile(filepath.Join(stage, "START-HERE.md"), []byte("changed stage"), 0600); err != nil {
		t.Fatal(err)
	}
	s.RetryUIReplacement()
	s.jobs.Wait()
	if s.UIReplacementStatus().State != "FAILED" || s.uiReplacement.directory != stage {
		t.Fatal("changed stage became applicable")
	}
	entries, err = os.ReadDir(filepath.Join(base, "Krasis", "updates"))
	if err != nil || len(entries) != 1 {
		t.Fatal("invalid stage silently restaged", err)
	}
}
