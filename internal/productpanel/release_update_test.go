package productpanel

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestPublicUpdateWaitsForDisconnectedInputAndOnlyLaunchesOnce(t *testing.T) {
	client := &fakeClient{state: "CONNECTED"}
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return client, nil }, nil, nil)
	launches := 0
	launch := func() error { launches++; return nil }
	if b.ApplyReleaseUpdate(launch) || launches != 0 {
		t.Fatal("interrupted active session")
	}
	client.state = "LOCAL"
	if b.ApplyReleaseUpdate(func() error { return errors.New("launch failed") }) || b.updateHandedOff {
		t.Fatal("failed launch consumed replacement")
	}
	if !b.ApplyReleaseUpdate(launch) || launches != 1 || !b.stopped.Load() {
		t.Fatal("idle replacement failed")
	}
	if b.ApplyReleaseUpdate(launch) || launches != 1 {
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
