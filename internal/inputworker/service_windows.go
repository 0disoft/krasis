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
	agent, service, err := installation.digests()
	if err != nil || !fixedInputProcess(service, false) {
		return ErrAuthority
	}
	managed, err := svc.IsWindowsService()
	if err != nil || !managed {
		return ErrAuthority
	}
	root, err := windows.KnownFolderPath(windows.FOLDERID_ProgramFiles, 0)
	if err != nil {
		return ErrAuthority
	}
	if svc.Run(ServiceName, &serviceHandler{check: func(args []string) error {
		return checkServiceStart(filepath.Join(root, "Krasis"), installImages{Agent: agent, Service: service}, args)
	}, run: func(ctx context.Context) error { return runInstalledService(ctx, installation, service) }}) != nil {
		return ErrAuthority
	}
	return nil
}

func fixedInputProcess(digest [32]byte, console bool) bool {
	return fixedInstalledInputProcess(digest, console, nil)
}

func fixedInstalledInputProcess(digest [32]byte, console bool, installation *Installation) bool {
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
	image, err := pinIdentityImage(installation, identity.image, digest)
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
	agent, service, err := installation.workerDigests()
	if err != nil || !fixedInstalledInputProcess(service, true, installation) {
		return ErrAuthority
	}
	// Include the load/hash work performed before entering this function. The
	// span overlaps ServiceChildConnect; it must not be added to that span.
	profile.Record(handoff.WorkerInstalledValidation, installation.loadElapsed+time.Since(started))
	profile.RecordLoad(installation.loadElapsed)
	return runInstalledConsoleInput(ctx, agent, service, profile, installation)
}

func runServiceLoop(ctx context.Context, installation *Installation, ready func() error) (result error) {
	agent, service, err := installation.digests()
	if err != nil {
		return ErrAuthority
	}
	var next *preparedConsole
	var lastPreparation time.Time
	prepareNext := false
	defer func() {
		if next.close() != nil {
			result = errors.Join(result, ErrRecovery)
		}
	}()
	for ctx.Err() == nil {
		readyFailed := false
		var preparationErr error
		channel, err := acceptAgentReady(ctx, agent, func() error {
			if ready != nil && ready() != nil {
				readyFailed = true
				return ErrAuthority
			}
			if prepareNext {
				prepareNext = false
				lastPreparation = time.Now()
				// The endpoint already exists: a quick next connection can wait
				// in the pipe while one unbound child prepares. No extra accept
				// failure or per-idle retry can create another warm child.
				next, preparationErr = prepareServiceConsole(ctx, installation, service)
				if errors.Is(preparationErr, ErrRecovery) {
					return ErrRecovery
				}
			}
			return nil
		}, installation)
		if errors.Is(preparationErr, ErrRecovery) {
			return ErrRecovery
		}
		if readyFailed {
			return ErrAuthority
		}
		if err == nil {
			lease, prepare, leaseErr := receiveAgentRequest(ctx, channel)
			if leaseErr == nil && prepare {
				// No lease is created and no agent handle is transferred. A
				// duplicate hint cannot extend expiry or replace a live child.
				if next == nil || next.retired() {
					if next.close() != nil {
						_ = channel.Close()
						return ErrRecovery
					}
					next = nil
					if preparationAllowed(lastPreparation, time.Now()) {
						lastPreparation = time.Now()
						next, err = prepareServiceConsole(ctx, installation, service)
					}
				}
				if err == nil && channel.authority.check(false) == nil {
					_ = channel.stream.SetWriteDeadline(time.Now().Add(time.Second))
					err = writeFrame(channel.stream, preparationReceipt[:])
				}
				closeErr := channel.Close()
				if errors.Is(err, ErrRecovery) || closeErr != nil {
					return ErrRecovery
				}
				// Preparation failures leave ordinary lease fallback usable.
				continue
			}
			if leaseErr == nil {
				candidate := next
				next = nil
				_, err = runPreparedAgentLease(ctx, channel, lease, agent, service, installation, candidate)
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
			prepareNext = true
			continue
		}
		if next.close() != nil {
			return ErrRecovery
		}
		next = nil
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

type serviceHandler struct {
	check func([]string) error
	run   func(context.Context) error
}

func (handler *serviceHandler) Execute(args []string, changes <-chan svc.ChangeRequest, updates chan<- svc.Status) (bool, uint32) {
	updates <- svc.Status{State: svc.StartPending, CheckPoint: 1, WaitHint: 10000}
	if handler.run == nil || handler.check == nil && len(args) != 1 {
		return true, 1
	}
	if handler.check != nil && handler.check(args) != nil {
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
