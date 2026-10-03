//go:build windows

package inputworker

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"time"

	"github.com/0disoft/krasis/internal/handoff"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
)

// RunService starts only through SCM with a protected installation. It neither
// installs itself nor elevates a portable agent.
func RunService(installation *Installation) error {
	_, service, err := installation.digests()
	if err != nil || !fixedInputProcess(service, false) {
		return ErrAuthority
	}
	managed, err := svc.IsWindowsService()
	if err != nil || !managed {
		return ErrAuthority
	}
	if svc.Run(ServiceName, &serviceHandler{run: func(ctx context.Context) error { return runInstalledService(ctx, installation, service) }}) != nil {
		return ErrAuthority
	}
	return nil
}

func fixedInputProcess(digest [32]byte, console bool) bool {
	identity, err := inspectProcess(windows.GetCurrentProcessId())
	if err != nil {
		return false
	}
	defer identity.close()
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil || !identity.system || identity.integrity != 0x4000 || identity.restricted || identity.appContainer || !strings.EqualFold(identity.image, filepath.Join(root, "Krasis", "Input", "krasis-input.exe")) {
		return false
	}
	if console {
		if !validConsoleToken(identity.token, windows.WTSGetActiveConsoleSessionId()) {
			return false
		}
	} else if identity.session != 0 {
		return false
	}
	image, err := lockImage(identity.image, digest)
	if err != nil {
		return false
	}
	_ = image.Close()
	return true
}

// RunInstalledWorker cannot consume a caller-provided trust record.
func RunInstalledWorker(ctx context.Context, installation *Installation) error {
	var profile handoff.StartupProfile
	started := time.Now()
	agent, service, err := installation.digests()
	if err != nil || !fixedInputProcess(service, true) {
		return ErrAuthority
	}
	profile.Record(handoff.WorkerInstalledValidation, time.Since(started))
	return runConsoleInput(ctx, agent, service, profile)
}

func runServiceLoop(ctx context.Context, installation *Installation, ready func() error) error {
	agent, service, err := installation.digests()
	if err != nil {
		return ErrAuthority
	}
	for ctx.Err() == nil {
		readyFailed := false
		channel, err := acceptAgentReady(ctx, agent, func() error {
			if ready != nil && ready() != nil {
				readyFailed = true
				return ErrAuthority
			}
			return nil
		})
		if readyFailed {
			return ErrAuthority
		}
		if err == nil {
			lease, leaseErr := receiveLease(ctx, channel)
			if leaseErr == nil {
				_, err = RunAgentLease(ctx, channel, lease, agent, service)
				if errors.Is(err, ErrRecovery) {
					_ = channel.Close()
					return ErrRecovery
				}
			} else {
				err = leaseErr
			}
			if closeErr := channel.Close(); closeErr != nil && err == nil {
				err = closeErr
			}
		}
		// Recovery and the owned worker have already finished. A clean lease
		// needs no retry backoff; the next accept still blocks awaiting a client.
		if err == nil {
			continue
		}
		timer := time.NewTimer(200 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}
	}
	return nil
}

type serviceHandler struct{ run func(context.Context) error }

func (handler *serviceHandler) Execute(args []string, changes <-chan svc.ChangeRequest, updates chan<- svc.Status) (bool, uint32) {
	updates <- svc.Status{State: svc.StartPending, CheckPoint: 1, WaitHint: 10000}
	if len(args) != 1 || handler.run == nil {
		return true, 1
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- handler.run(ctx) }()
	current := svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	updates <- current
	for {
		select {
		case err := <-done:
			if err != nil {
				return true, 2
			}
			return false, 0
		case request, ok := <-changes:
			if ok && request.Cmd == svc.Interrogate {
				updates <- current
				continue
			}
			if ok && request.Cmd != svc.Stop && request.Cmd != svc.Shutdown {
				continue
			}
			cancel()
			current = svc.Status{State: svc.StopPending, CheckPoint: 1, WaitHint: 10000}
			updates <- current
			ticker := time.NewTicker(time.Second)
			defer ticker.Stop()
			for {
				select {
				case err := <-done:
					if err != nil {
						return true, 2
					}
					return false, 0
				case <-ticker.C:
					current.CheckPoint++
					updates <- current
				case request, ok := <-changes:
					if !ok {
						changes = nil
						continue
					}
					if request.Cmd == svc.Interrogate {
						updates <- current
					}
				}
			}
		}
	}
}
