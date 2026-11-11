//go:build linux

package launcher

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productcli"
)

type fakeWatchClient struct {
	status       localapi.Status
	statusSchema func() localapi.Status
	statusErr    error
	trust        localapi.Trust
	trustErr     error
	resumeCalls  int
	resumeErr    error
	resumeErrAt  func(int) error
	fingerprint  string
}

func (client *fakeWatchClient) Status(context.Context) (localapi.Status, error) {
	if client.statusSchema != nil {
		return client.statusSchema(), client.statusErr
	}
	return client.status, client.statusErr
}

func (client *fakeWatchClient) Trust(context.Context) (localapi.Trust, error) {
	return client.trust, client.trustErr
}

func (client *fakeWatchClient) ResumeConnection(ctx context.Context, _ string) (localapi.OperationResult, error) {
	client.resumeCalls++
	client.fingerprint = localapi.PeerFingerprint(ctx)
	if client.resumeErrAt != nil {
		return localapi.OperationResult{Status: "COMPLETED", Code: "connect_started"}, client.resumeErrAt(client.resumeCalls)
	}
	return localapi.OperationResult{Status: "COMPLETED", Code: "connect_started"}, client.resumeErr
}

func TestWatchReconcileResumesOnlySavedTrustedPeer(t *testing.T) {
	base := t.TempDir()
	fingerprint := strings.Repeat("a", 64)
	if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	client := &fakeWatchClient{
		status: localapi.Status{State: "LOCAL"},
		trust: localapi.Trust{Peers: []localapi.TrustedPeer{{
			Alias: "B", Fingerprint: fingerprint,
		}}},
	}
	ensured := 0
	dependencies := watchTestDependencies(base, client, func(context.Context) (bool, error) { return true, nil })
	dependencies.ensure = func(context.Context, string, string) error { ensured++; return nil }
	if _, _, err := reconcileWatch(context.Background(), base, "/opt/krasis-cli", true, dependencies); err != nil {
		t.Fatal(err)
	}
	if ensured != 0 || client.resumeCalls != 1 || client.fingerprint != fingerprint {
		t.Fatalf("ensured=%d resumes=%d fingerprint=%q", ensured, client.resumeCalls, client.fingerprint)
	}
	stored, err := appsettings.LoadResume(base)
	if err != nil || stored.Alias != "B" || stored.Fingerprint != fingerprint || stored.ManualStop {
		t.Fatalf("saved resume changed: %+v, %v", stored, err)
	}
}

func TestWatchReconcileStartsTrustedSiblingAndStopsOnNewLock(t *testing.T) {
	base := t.TempDir()
	fingerprint := strings.Repeat("b", 64)
	if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	client := &fakeWatchClient{
		status: localapi.Status{State: "LOCAL"},
		trust: localapi.Trust{Peers: []localapi.TrustedPeer{{
			Alias: "B", Fingerprint: fingerprint,
		}}},
	}
	probes := 0
	dependencies := watchTestDependencies(base, client, func(context.Context) (bool, error) {
		probes++
		return probes < 2, nil
	})
	opens := 0
	dependencies.open = func(string) (watchClient, error) {
		opens++
		if opens == 1 {
			return nil, localapi.ErrUnavailable
		}
		return client, nil
	}
	ensured := 0
	dependencies.ensure = func(context.Context, string, string) error { ensured++; return nil }
	if _, _, err := reconcileWatch(context.Background(), base, "/opt/krasis-cli", true, dependencies); err != nil {
		t.Fatal(err)
	}
	if ensured != 1 || client.resumeCalls != 0 {
		t.Fatalf("ensured=%d resumes=%d", ensured, client.resumeCalls)
	}
}

func TestWatchReconcileHonorsManualStopAndSafeStopPause(t *testing.T) {
	fingerprint := strings.Repeat("c", 64)
	for _, test := range []struct {
		name   string
		resume appsettings.Resume
		pause  bool
	}{
		{name: "manual stop", resume: appsettings.Resume{ManualStop: true}},
		{name: "safe stop", resume: appsettings.Resume{Alias: "B", Fingerprint: fingerprint}, pause: true},
		{name: "no saved peer"},
	} {
		t.Run(test.name, func(t *testing.T) {
			base := t.TempDir()
			if err := appsettings.SaveResume(base, test.resume); err != nil {
				t.Fatal(err)
			}
			if test.pause {
				if err := appsettings.PauseRecovery(base, true); err != nil {
					t.Fatal(err)
				}
			}
			opened, ensured := 0, 0
			client := &fakeWatchClient{status: localapi.Status{State: "LOCAL"}}
			dependencies := watchTestDependencies(base, client, func(context.Context) (bool, error) { return true, nil })
			dependencies.open = func(string) (watchClient, error) { opened++; return client, nil }
			dependencies.ensure = func(context.Context, string, string) error { ensured++; return nil }
			if _, _, err := reconcileWatch(context.Background(), base, "/opt/krasis-cli", true, dependencies); err != nil {
				t.Fatal(err)
			}
			if opened != 0 || ensured != 0 || client.resumeCalls != 0 {
				t.Fatalf("opened=%d ensured=%d resumes=%d", opened, ensured, client.resumeCalls)
			}
			stored, err := appsettings.LoadResume(base)
			if err != nil || stored.ManualStop != test.resume.ManualStop || stored.Alias != test.resume.Alias || stored.Fingerprint != test.resume.Fingerprint {
				t.Fatalf("saved resume changed: %+v, %v", stored, err)
			}
		})
	}
}

func TestWatchReconcileRejectsRevokedOrChangedTrust(t *testing.T) {
	for _, peer := range []localapi.TrustedPeer{
		{Alias: "B", Fingerprint: strings.Repeat("d", 64), Revoked: true},
		{Alias: "B", Fingerprint: strings.Repeat("d", 64)},
	} {
		base := t.TempDir()
		fingerprint := strings.Repeat("c", 64)
		if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: fingerprint}); err != nil {
			t.Fatal(err)
		}
		client := &fakeWatchClient{status: localapi.Status{State: "LOCAL"}, trust: localapi.Trust{Peers: []localapi.TrustedPeer{peer}}}
		dependencies := watchTestDependencies(base, client, func(context.Context) (bool, error) { return true, nil })
		if _, _, err := reconcileWatch(context.Background(), base, "/opt/krasis-cli", true, dependencies); err != nil {
			t.Fatal(err)
		}
		if client.resumeCalls != 0 {
			t.Fatal("changed or revoked peer was resumed")
		}
	}
}

func TestWatchLoopAttemptsOncePerSafeTransition(t *testing.T) {
	base := t.TempDir()
	fingerprint := strings.Repeat("e", 64)
	if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	client := &fakeWatchClient{
		status: localapi.Status{State: "LOCAL"},
		trust: localapi.Trust{Peers: []localapi.TrustedPeer{{
			Alias: "B", Fingerprint: fingerprint,
		}}},
		resumeErr: errors.New("bounded resume outcome unknown"),
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waits := 0
	dependencies := watchTestDependencies(base, client, func(context.Context) (bool, error) { return true, nil })
	dependencies.wait = func(ctx context.Context, _ time.Duration) error {
		waits++
		if waits == 3 {
			cancel()
		}
		return ctx.Err()
	}
	if err := runWatchLoop(ctx, base, "/opt/krasis-cli", dependencies); err != nil {
		t.Fatal(err)
	}
	if client.resumeCalls != 1 {
		t.Fatalf("ambiguous resume replayed %d times", client.resumeCalls)
	}
}

func TestWatchLoopRetriesAfterUnsafeTransition(t *testing.T) {
	base := t.TempDir()
	fingerprint := strings.Repeat("f", 64)
	if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	client := &fakeWatchClient{
		status: localapi.Status{State: "LOCAL"},
		trust: localapi.Trust{Peers: []localapi.TrustedPeer{{
			Alias: "B", Fingerprint: fingerprint,
		}}},
	}
	sequence := []bool{true, true, false, true, true}
	probes := 0
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waits := 0
	dependencies := watchTestDependencies(base, client, func(context.Context) (bool, error) {
		ready := true
		if probes < len(sequence) {
			ready = sequence[probes]
		}
		probes++
		return ready, nil
	})
	dependencies.wait = func(ctx context.Context, _ time.Duration) error {
		waits++
		if waits == 3 {
			cancel()
		}
		return ctx.Err()
	}
	if err := runWatchLoop(ctx, base, "/opt/krasis-cli", dependencies); err != nil {
		t.Fatal(err)
	}
	if client.resumeCalls != 2 {
		t.Fatalf("resumes after safe transitions = %d", client.resumeCalls)
	}
}

func TestWatchLoopRearmsAfterConnectedAndResumesPeerEOF(t *testing.T) {
	base := t.TempDir()
	fingerprint := strings.Repeat("1", 64)
	if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	client := &fakeWatchClient{
		trust: localapi.Trust{Peers: []localapi.TrustedPeer{{
			Alias: "B", Fingerprint: fingerprint,
		}}},
	}
	states := []string{"LOCAL", "CONNECTED", "LOCAL"}
	statuses := 0
	client.statusSchema = func() localapi.Status {
		state := states[len(states)-1]
		if statuses < len(states) {
			state = states[statuses]
		}
		statuses++
		return localapi.Status{State: state}
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waits := 0
	dependencies := watchTestDependencies(base, client, func(context.Context) (bool, error) { return true, nil })
	dependencies.wait = func(ctx context.Context, _ time.Duration) error {
		waits++
		if waits == 3 {
			cancel()
		}
		return ctx.Err()
	}
	if err := runWatchLoop(ctx, base, "/opt/krasis-cli", dependencies); err != nil {
		t.Fatal(err)
	}
	if client.resumeCalls != 2 {
		t.Fatalf("resumes after peer EOF = %d", client.resumeCalls)
	}
}

func TestWatchLoopRetriesTransientResumeErrorWithinBound(t *testing.T) {
	base := t.TempDir()
	fingerprint := strings.Repeat("2", 64)
	if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	client := &fakeWatchClient{
		status: localapi.Status{State: "LOCAL"},
		trust: localapi.Trust{Peers: []localapi.TrustedPeer{{
			Alias: "B", Fingerprint: fingerprint,
		}}},
		resumeErrAt: func(attempt int) error {
			if attempt < 3 {
				return localapi.ErrTransport
			}
			return nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waits := 0
	dependencies := watchTestDependencies(base, client, func(context.Context) (bool, error) { return true, nil })
	dependencies.wait = func(ctx context.Context, _ time.Duration) error {
		waits++
		if waits == 6 {
			cancel()
		}
		return ctx.Err()
	}
	if err := runWatchLoop(ctx, base, "/opt/krasis-cli", dependencies); err != nil {
		t.Fatal(err)
	}
	if client.resumeCalls != 3 {
		t.Fatalf("transient resume attempts = %d, want 3", client.resumeCalls)
	}
}

func TestWatchLoopCapsPersistentResumeErrors(t *testing.T) {
	base := t.TempDir()
	fingerprint := strings.Repeat("3", 64)
	if err := appsettings.SaveResume(base, appsettings.Resume{Alias: "B", Fingerprint: fingerprint}); err != nil {
		t.Fatal(err)
	}
	client := &fakeWatchClient{
		status:    localapi.Status{State: "LOCAL"},
		trust:     localapi.Trust{Peers: []localapi.TrustedPeer{{Alias: "B", Fingerprint: fingerprint}}},
		resumeErr: localapi.ErrTransport,
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	waits := 0
	dependencies := watchTestDependencies(base, client, func(context.Context) (bool, error) { return true, nil })
	dependencies.wait = func(ctx context.Context, _ time.Duration) error {
		waits++
		if waits == 8 {
			cancel()
		}
		return ctx.Err()
	}
	if err := runWatchLoop(ctx, base, "/opt/krasis-cli", dependencies); err != nil {
		t.Fatal(err)
	}
	if client.resumeCalls != 3 {
		t.Fatalf("persistent resume attempted %d times, want cap 3", client.resumeCalls)
	}
}

func TestWatchCLIIsOptInAndRejectsControlOverride(t *testing.T) {
	for _, args := range [][]string{{"watch"}, {"--json", "watch"}, {"watch", "--json"}} {
		var output, failure bytes.Buffer
		watches := 0
		code := runProductCLIWithWatch(context.Background(), args, &output, &failure, t.TempDir(), func(context.Context, string, string) error {
			return nil
		}, func(context.Context, string, bool, func(context.Context, string, string) error) error {
			watches++
			return nil
		})
		if code != productcli.ExitOK || watches != 1 || failure.Len() != 0 {
			t.Fatalf("%v: code=%d watches=%d stderr=%q", args, code, watches, failure.String())
		}
	}
	var output, failure bytes.Buffer
	watches := 0
	code := runProductCLIWithWatch(context.Background(), []string{"--control-dir", "/tmp/control", "watch"}, &output, &failure, t.TempDir(), func(context.Context, string, string) error {
		return nil
	}, func(context.Context, string, bool, func(context.Context, string, string) error) error {
		watches++
		return nil
	})
	if code != productcli.ExitUsage || watches != 0 || failure.Len() == 0 {
		t.Fatalf("override: code=%d watches=%d stderr=%q", code, watches, failure.String())
	}
	output.Reset()
	failure.Reset()
	watches = 0
	code = runProductCLIWithWatch(context.Background(), []string{"agent", "watch"}, &output, &failure, t.TempDir(), func(context.Context, string, string) error {
		return nil
	}, func(context.Context, string, bool, func(context.Context, string, string) error) error {
		watches++
		return nil
	})
	if code == productcli.ExitOK || watches != 0 {
		t.Fatalf("unapproved agent watch alias: code=%d watches=%d", code, watches)
	}
}

func watchTestDependencies(base string, client watchClient, available func(context.Context) (bool, error)) watchDependencies {
	return watchDependencies{
		available:      available,
		loadResume:     appsettings.LoadResume,
		recoveryPaused: appsettings.RecoveryPaused,
		resolveControl: func(string) (string, error) { return "/control", nil },
		open:           func(string) (watchClient, error) { return client, nil },
		ensure:         func(context.Context, string, string) error { return nil },
		wait:           waitContext,
	}
}
