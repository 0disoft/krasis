//go:build linux

package launcher

import (
	"context"
	"errors"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0disoft/krasis/internal/agentinstance"
	"github.com/0disoft/krasis/internal/appsettings"
	"github.com/0disoft/krasis/internal/localapi"
	"github.com/0disoft/krasis/internal/productcli"
	"github.com/0disoft/krasis/internal/winlifecycle"
)

const (
	watchPollInterval   = 2 * time.Second
	watchMaxBackoff     = 10 * time.Second
	watchActionBudget   = 20 * time.Second
	watchResumeAttempts = 3
)

type watchClient interface {
	Status(context.Context) (localapi.Status, error)
	Trust(context.Context) (localapi.Trust, error)
	ResumeConnection(context.Context, string) (localapi.OperationResult, error)
}

type watchDependencies struct {
	available      func(context.Context) (bool, error)
	loadResume     func(string) (appsettings.Resume, error)
	recoveryPaused func(string) (bool, error)
	resolveControl func(string) (string, error)
	open           func(string) (watchClient, error)
	ensure         func(context.Context, string, string) error
	wait           func(context.Context, time.Duration) error
}

func defaultWatchDependencies(ensure func(context.Context, string, string) error) watchDependencies {
	return watchDependencies{
		available:      winlifecycle.LinuxSessionReady,
		loadResume:     appsettings.LoadResume,
		recoveryPaused: appsettings.RecoveryPaused,
		resolveControl: func(localData string) (string, error) { return productcli.ResolveControlDirectory(localData, "") },
		open:           func(control string) (watchClient, error) { return localapi.OpenClient(control) },
		ensure:         ensure,
		wait:           waitContext,
	}
}

func runLinuxWatch(ctx context.Context, localData string, _ bool, ensure func(context.Context, string, string) error) error {
	if ctx == nil || localData == "" || ensure == nil {
		return errors.New("Linux watch configuration is incomplete")
	}
	lease, err := agentinstance.AcquireWatch()
	if err != nil {
		return err
	}
	defer lease.Close()
	executable, err := os.Executable()
	if err != nil {
		return err
	}
	watchContext, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	return runWatchLoop(watchContext, localData, executable, defaultWatchDependencies(ensure))
}

// A watcher polls the resident agent while logind remains safe so a clean peer
// EOF is recoverable without a local lifecycle transition. Probe and action
// failures back off monotonically. A failed dispatched resume is confirmed with
// a later LOCAL observation and retried at most three times per safe episode.
func runWatchLoop(ctx context.Context, localData, executable string, dependencies watchDependencies) error {
	if ctx == nil || localData == "" || executable == "" || !validWatchDependencies(dependencies) {
		return errors.New("Linux watch dependencies are incomplete")
	}
	safe := false
	resumeArmed := true
	resumeRetryPending := false
	resumeAttempts := 0
	backoff := watchPollInterval
	for {
		if err := ctx.Err(); err != nil {
			return nil
		}
		ready, err := dependencies.available(ctx)
		if err != nil || !ready {
			safe, resumeArmed = false, true
			resumeRetryPending, resumeAttempts = false, 0
			if err != nil {
				backoff *= 2
				if backoff > watchMaxBackoff {
					backoff = watchMaxBackoff
				}
			}
		} else {
			if !safe {
				safe, resumeArmed = true, true
			}
			state, resumed, actionErr := reconcileWatch(ctx, localData, executable, resumeArmed, dependencies)
			if resumed {
				resumeArmed = false
				resumeAttempts++
				if (errors.Is(actionErr, localapi.ErrTransport) || errors.Is(actionErr, localapi.ErrUnavailable)) && resumeAttempts < watchResumeAttempts {
					resumeRetryPending = true
				} else {
					resumeRetryPending = false
				}
			}
			if state != "" && state != "LOCAL" {
				resumeArmed = true
				resumeRetryPending, resumeAttempts = false, 0
			} else if !resumed && state == "LOCAL" && resumeRetryPending && resumeAttempts < watchResumeAttempts {
				resumeArmed = true
				resumeRetryPending = false
			}
			if actionErr != nil {
				err = actionErr
				backoff *= 2
				if backoff > watchMaxBackoff {
					backoff = watchMaxBackoff
				}
			} else {
				backoff = watchPollInterval
			}
		}
		delay := watchPollInterval
		if err != nil {
			delay = backoff
		}
		if err := dependencies.wait(ctx, delay); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
	}
}

func reconcileWatch(ctx context.Context, localData, executable string, allowResume bool, dependencies watchDependencies) (string, bool, error) {
	actionContext, cancel := context.WithTimeout(ctx, watchActionBudget)
	defer cancel()
	peer, err := dependencies.loadResume(localData)
	if err != nil || peer.ManualStop || peer.Alias == "" || peer.Fingerprint == "" {
		return "", false, err
	}
	paused, err := dependencies.recoveryPaused(localData)
	if err != nil || paused {
		return "", false, err
	}
	control, err := dependencies.resolveControl(localData)
	if err != nil {
		return "", false, err
	}
	client, status, err := readyWatchClient(actionContext, localData, executable, control, dependencies)
	if err != nil || client == nil {
		return status.State, false, err
	}
	if status.State != "LOCAL" {
		return status.State, false, nil
	}
	trust, err := client.Trust(actionContext)
	if err != nil || !watchPeerTrusted(trust, peer.Alias, peer.Fingerprint) {
		return status.State, false, err
	}
	if ready, err := dependencies.available(actionContext); err != nil || !ready {
		return status.State, false, err
	}
	if !allowResume {
		return status.State, false, nil
	}
	resumeContext := localapi.WithPeerFingerprint(actionContext, peer.Fingerprint)
	_, err = client.ResumeConnection(resumeContext, peer.Alias)
	return status.State, true, err
}

func readyWatchClient(ctx context.Context, localData, executable, control string, dependencies watchDependencies) (watchClient, localapi.Status, error) {
	client, status, err := openWatchClient(ctx, control, dependencies.open)
	if err == nil {
		return client, status, nil
	}
	if !errors.Is(err, localapi.ErrUnavailable) && !errors.Is(err, localapi.ErrTransport) {
		return nil, localapi.Status{}, err
	}
	if ready, probeErr := dependencies.available(ctx); probeErr != nil || !ready {
		return nil, localapi.Status{}, probeErr
	}
	if err := dependencies.ensure(ctx, localData, executable); err != nil {
		return nil, localapi.Status{}, err
	}
	if ready, probeErr := dependencies.available(ctx); probeErr != nil || !ready {
		return nil, localapi.Status{}, probeErr
	}
	return openWatchClient(ctx, control, dependencies.open)
}

func openWatchClient(ctx context.Context, control string, open func(string) (watchClient, error)) (watchClient, localapi.Status, error) {
	client, err := open(control)
	if err != nil {
		return nil, localapi.Status{}, err
	}
	status, err := client.Status(ctx)
	if err != nil {
		return nil, localapi.Status{}, err
	}
	return client, status, nil
}

func watchPeerTrusted(trust localapi.Trust, alias, fingerprint string) bool {
	matches := 0
	for _, peer := range trust.Peers {
		if peer.Alias != alias {
			continue
		}
		if peer.Revoked || peer.Fingerprint != fingerprint {
			return false
		}
		matches++
	}
	return matches == 1
}

func validWatchDependencies(d watchDependencies) bool {
	return d.available != nil && d.loadResume != nil && d.recoveryPaused != nil && d.resolveControl != nil && d.open != nil && d.ensure != nil && d.wait != nil
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}
