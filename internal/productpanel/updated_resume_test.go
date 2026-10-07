package productpanel

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/localapi"
)

type resumeTestClient struct {
	*fakeClient
	info      localapi.RuntimeInfo
	rejects   int
	uncertain bool
}

func (c *resumeTestClient) Runtime(context.Context) (localapi.RuntimeInfo, error) { return c.info, nil }
func (c *resumeTestClient) Status(ctx context.Context) (localapi.Status, error) {
	v, err := c.fakeClient.Status(ctx)
	if v.State == "CONNECTED" || v.State == "CONNECTING" || v.State == "RECONNECTING" {
		v.PeerAlias = "B"
	}
	return v, err
}
func (c *resumeTestClient) Connect(ctx context.Context, alias string) (localapi.OperationResult, error) {
	if c.uncertain {
		c.connects++
		return localapi.OperationResult{}, errors.New("lost response")
	}
	if c.rejects > 0 {
		c.rejects--
		c.connects++
		return localapi.OperationResult{Status: "REJECTED"}, nil
	}
	return c.fakeClient.Connect(ctx, alias)
}

func newResumeTest(t *testing.T, ensure func(context.Context) error) (*DesktopLifecycle, *resumeTestClient, time.Time) {
	t.Helper()
	revision, fp := strings.Repeat("a", 40), strings.Repeat("b", 64)
	c := &resumeTestClient{fakeClient: &fakeClient{state: "LOCAL", peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fp}}}, info: localapi.RuntimeInfo{Protocol: localapi.AgentProtocol, Generation: localapi.AgentGeneration, Build: revision}}
	if ensure == nil {
		ensure = func(context.Context) error { return nil }
	}
	b := NewDesktopLifecycle(context.Background(), func() (Client, error) { return c, nil }, ensure, nil)
	if err := b.SetUpdatedWorkspaceResume(WorkspaceUpdateLaunch{Alias: "B", Fingerprint: fp, Revision: revision}); err != nil {
		t.Fatal(err)
	}
	return b, c, time.Now()
}

func TestUpdateResumeSurvivesBusyStartupAndHandsOffAcceptedConnection(t *testing.T) {
	starts := 0
	b, c, now := newResumeTest(t, func(context.Context) error {
		starts++
		if starts == 1 {
			return ErrStartupBusy
		}
		return nil
	})
	b.tick(now)
	if b.Refresh().Code != "startup_busy" || b.updatedResume == nil || c.connects != 0 {
		t.Fatal(b.Refresh(), c.connects)
	}
	b.tick(now.Add(time.Second))
	if starts != 1 {
		t.Fatal("busy startup retried without backoff")
	}
	b.tick(now.Add(6 * time.Second))
	if c.connects != 1 || b.Refresh().State != "CONNECTING" || b.updatedResume == nil {
		t.Fatal(b.Refresh(), c.connects)
	}
	b.tick(now.Add(12 * time.Second))
	if c.connects != 1 || starts != 2 {
		t.Fatal("accepted connect/startup replayed")
	}
	c.state = "CONNECTED"
	b.tick(now.Add(15 * time.Second))
	if b.updatedResume != nil || b.recovery.Alias != "B" || b.recovery.Fingerprint != c.peers[0].Fingerprint {
		t.Fatal("connected request not handed to ordinary recovery")
	}
}

func TestUpdateResumeRetriesKnownRejectionButBoundsAttemptsAndTime(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	c.rejects = 10
	for i := 0; i < 5; i++ {
		b.tick(now.Add(time.Duration(i) * 6 * time.Second))
	}
	if c.connects != 3 || b.updatedResume != nil || b.Refresh().Code != "recovery_exhausted" {
		t.Fatal(c.connects, b.Refresh())
	}
	b, c, now = newResumeTest(t, nil)
	b.tick(now.Add(91 * time.Second))
	if c.connects != 0 || b.updatedResume != nil || b.Refresh().Code != "recovery_exhausted" {
		t.Fatal("expired request executed")
	}
}

func TestUpdateResumeNeverReplaysAnUncertainConnect(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	c.uncertain = true
	b.tick(now)
	for i := 1; i < 5; i++ {
		b.tick(now.Add(time.Duration(i) * 6 * time.Second))
	}
	if c.connects != 1 || b.Refresh().Code != "outcome_unknown" {
		t.Fatal(c.connects, b.Refresh())
	}
	c.state = "CONNECTED"
	b.tick(now.Add(30 * time.Second))
	if b.updatedResume != nil || c.connects != 1 {
		t.Fatal("read-only confirmation failed")
	}
}

func TestUpdateResumeRejectsChangedTrustBuildAndPermanentStartupFailure(t *testing.T) {
	for _, mode := range []string{"fingerprint", "revoked", "duplicate", "build", "generation", "protocol", "permanent"} {
		t.Run(mode, func(t *testing.T) {
			b, c, now := newResumeTest(t, nil)
			switch mode {
			case "fingerprint":
				c.peers[0].Fingerprint = strings.Repeat("c", 64)
			case "revoked":
				c.peers[0].Revoked = true
			case "duplicate":
				c.peers = append(c.peers, c.peers[0])
			case "build":
				c.info.Build = strings.Repeat("c", 40)
			case "generation":
				c.info.Generation--
			case "protocol":
				c.info.Protocol++
			case "permanent":
				b.ensure = func(context.Context) error { return ErrAgentRecoveryFailed }
			}
			b.tick(now)
			b.tick(now.Add(6 * time.Second))
			if c.connects != 0 || b.updatedResume != nil {
				t.Fatal(mode, c.connects, b.Refresh())
			}
		})
	}
}

func TestUpdateResumeStopsOnUserDisconnectAndDuringInflightStartup(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	b.Disconnect()
	b.tick(now)
	if c.connects != 0 || b.updatedResume != nil {
		t.Fatal("manual disconnect ignored")
	}
	entered := make(chan struct{})
	b, c, now = newResumeTest(t, func(ctx context.Context) error { close(entered); <-ctx.Done(); return ctx.Err() })
	done := make(chan struct{})
	go func() { b.tick(now); close(done) }()
	<-entered
	b.Disconnect()
	<-done
	b.tick(now.Add(6 * time.Second))
	if c.connects != 0 || b.updatedResume != nil {
		t.Fatal("canceled startup reconnected")
	}
}

func TestUpdateResumeDoesNotCreateSessionForPublicRelease(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	b.updatedResume = nil
	if err := b.SetUpdatedWorkspaceResume(WorkspaceUpdateLaunch{Revision: c.info.Build}); err != nil {
		t.Fatal(err)
	}
	b.tick(now)
	if c.connects != 0 || b.updatedResume != nil {
		t.Fatal("public release invented peer")
	}
}

func TestUpdateResumeRemainsPendingAfterInitialStartupBusy(t *testing.T) {
	starts := 0
	b, c, now := newResumeTest(t, func(context.Context) error {
		starts++
		if starts == 1 {
			return ErrStartupBusy
		}
		return nil
	})
	b.initialize(context.Background(), NewDesktopBridge(context.Background(), b.open).Refresh())
	if b.updatedResume == nil || b.Refresh().Code != "startup_busy" {
		t.Fatal("initial startup lost the launch identity")
	}
	b.tick(now)
	if c.connects != 1 || starts != 2 {
		t.Fatal("pending update did not resume after startup recovered", c.connects, starts)
	}
}

func TestUpdateResumeRechecksTrustAfterStartupAndStopsWithOwner(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	b.ensure = func(context.Context) error { c.peers[0].Fingerprint = strings.Repeat("c", 64); return nil }
	b.tick(now)
	if c.connects != 0 || b.updatedResume != nil {
		t.Fatal("trust changed during startup but CONNECT was sent")
	}
	b, c, now = newResumeTest(t, nil)
	ctx, cancel := context.WithCancel(context.Background())
	b.ctx = ctx
	cancel()
	b.tick(now)
	if c.connects != 0 || b.updatedResume != nil {
		t.Fatal("shutdown replayed update resume")
	}
}

func TestUpdateResumePreservesLegacyPinnedIdentityAndTransientTrustFailures(t *testing.T) {
	b, c, now := newResumeTest(t, nil)
	c.trustErr = errors.New("temporarily unreadable")
	b.tick(now)
	if b.updatedResume == nil || c.connects != 0 {
		t.Fatal("temporary trust read lost resume")
	}
	c.trustErr = nil
	b.tick(now.Add(6 * time.Second))
	if c.connects != 1 {
		t.Fatal("trust read recovery did not resume")
	}
	b, c, now = newResumeTest(t, nil)
	b.updatedResume.launch.Fingerprint = ""
	c.rejects = 1
	b.tick(now)
	c.peers[0].Fingerprint = strings.Repeat("c", 64)
	b.tick(now.Add(6 * time.Second))
	if c.connects != 1 || b.updatedResume != nil {
		t.Fatal("legacy peer was rebound during retry")
	}
}
