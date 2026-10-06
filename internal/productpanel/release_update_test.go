package productpanel

import (
	"context"
	"errors"
	"github.com/0disoft/krasis/internal/localapi"
	"strings"
	"testing"
)

type releaseClient struct {
	*fakeClient
	refuse   bool
	claimed  bool
	releases int
}

func (c *releaseClient) Runtime(context.Context) (localapi.RuntimeInfo, error) {
	return localapi.RuntimeInfo{Instance: strings.Repeat("a", 32)}, nil
}
func (c *releaseClient) ReleaseClaim(_ context.Context, _, _ string, _ int, _ string, claim bool) (localapi.OperationResult, error) {
	if claim && c.refuse {
		return localapi.OperationResult{Status: "REJECTED", Code: "agent_busy"}, nil
	}
	c.claimed = claim
	code := "release_claimed"
	if !claim {
		c.releases++
		code = "release_released"
	}
	return localapi.OperationResult{Status: "COMPLETED", Code: code}, nil
}

func TestPublicUpdateWaitsForDisconnectedInputAndOnlyLaunchesOnce(t *testing.T) {
	client := &releaseClient{fakeClient: &fakeClient{state: "CONNECTED"}}
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, nil, nil)
	launches := 0
	launch := func() error { launches++; return nil }
	revision := strings.Repeat("b", 40)
	if b.ApplyReleaseUpdate(999, revision, launch) || launches != 0 {
		t.Fatal("interrupted active session")
	}
	client.state = "LOCAL"
	if b.ApplyReleaseUpdate(999, revision, func() error { return errors.New("launch failed") }) || b.updateHandedOff || client.claimed || client.releases != 1 {
		t.Fatal("failed launch consumed replacement")
	}
	client.refuse = true
	if b.ApplyReleaseUpdate(999, revision, launch) || launches != 0 {
		t.Fatal("connected through CLI during idle check")
	}
	client.refuse = false
	if !b.ApplyReleaseUpdate(999, revision, launch) || launches != 1 || !b.stopped.Load() || !client.claimed {
		t.Fatal("idle replacement failed")
	}
	if b.ApplyReleaseUpdate(999, revision, launch) || launches != 1 {
		t.Fatal("duplicate public replacement")
	}
}
func TestStandaloneReleaseArgumentsDoNotRequirePeer(t *testing.T) {
	rev := strings.Repeat("a", 40)
	v, err := ParseWorkspaceUpdateLaunch([]string{"--release-build", rev, "--wait-pid", "123"})
	if err != nil || v.Alias != "" || v.Revision != rev || v.WaitPID != 123 {
		t.Fatalf("%+v %v", v, err)
	}
	for _, args := range [][]string{{"--release-build", rev, "--wait-pid", "0"}, {"--release-build", "bad", "--wait-pid", "123"}, {"--release-build", rev}} {
		if _, err = ParseWorkspaceUpdateLaunch(args); err == nil {
			t.Fatal(args)
		}
	}
}
